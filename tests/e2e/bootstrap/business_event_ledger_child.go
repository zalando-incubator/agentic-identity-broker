package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const LedgerChildEnvironment = "AIB_LEDGER_CHILD"

type LedgerChildMessage struct {
	State      string `json:"state"`
	AdminURL   string `json:"admin_url,omitempty"`
	EndUserURL string `json:"enduser_url,omitempty"`
}

type LedgerChild struct {
	cmd       *exec.Cmd
	input     io.WriteCloser
	encoder   *json.Encoder
	mu        sync.Mutex
	messages  chan LedgerChildMessage
	readError chan error
	done      chan error
	killOnce  sync.Once
	killError error
}

// StartLedgerChild runs the same test binary's TestLedgerChild entrypoint.
// Database credentials travel through stdin, not argv or child output.
func StartLedgerChild(ctx context.Context, executable string, config *ports.Config) (*LedgerChild, error) {
	if config == nil || config.Storage.Backend != "postgres" {
		return nil, errors.New("ledger crash proof requires PostgreSQL child configuration")
	}
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestLedgerChild$") // #nosec G204 -- the caller supplies its own test executable.
	cmd.Env = append(os.Environ(), LedgerChildEnvironment+"=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, errors.New("cannot start ledger child")
	}
	child := &LedgerChild{cmd: cmd, input: input, encoder: json.NewEncoder(input), messages: make(chan LedgerChildMessage, 4), readError: make(chan error, 1), done: make(chan error, 1)}
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var message LedgerChildMessage
			if err := decoder.Decode(&message); err != nil {
				child.readError <- errors.New("ledger child protocol ended")
				return
			}
			select {
			case child.messages <- message:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { child.done <- cmd.Wait() }()
	if err := child.encoder.Encode(config); err != nil {
		_ = child.Kill()
		return nil, errors.New("cannot configure ledger child")
	}
	return child, nil
}

func (c *LedgerChild) Await(ctx context.Context, state string) (LedgerChildMessage, error) {
	select {
	case message := <-c.messages:
		if message.State != state {
			return LedgerChildMessage{}, errors.New("unexpected ledger child state")
		}
		return message, nil
	case <-c.readError:
		return LedgerChildMessage{}, errors.New("ledger child protocol failed")
	case <-ctx.Done():
		return LedgerChildMessage{}, ctx.Err()
	}
}

func (c *LedgerChild) Send(command string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.encoder.Encode(command)
}

func (c *LedgerChild) CloseInput() error {
	return c.input.Close()
}

func (c *LedgerChild) Kill() error {
	c.killOnce.Do(func() {
		if err := c.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			c.killError = err
			return
		}
		_ = c.input.Close()
		select {
		case err := <-c.done:
			var exitError *exec.ExitError
			if err != nil && !errors.As(err, &exitError) {
				c.killError = err
			}
		case <-time.After(5 * time.Second):
			c.killError = errors.New("ledger child did not exit after kill")
		}
	})
	return c.killError
}

// RunLedgerChild is called only by the child test entrypoint. All HTTP routing
// and domain behavior come from the production builder and storage adapter.
func RunLedgerChild(input io.Reader, output io.Writer) error {
	decoder := json.NewDecoder(input)
	var config ports.Config
	if err := decoder.Decode(&config); err != nil {
		return errors.New("invalid ledger child configuration")
	}
	if config.Storage.Backend != "postgres" {
		return errors.New("ledger child requires PostgreSQL")
	}
	adapter, err := storageadapter.NewAdapter(&config.Storage)
	if err != nil {
		return errors.New("ledger child cannot open storage")
	}
	faults := &LedgerStorageFaults{}
	h := &LedgerHarness{Storage: adapter, ConnectionURL: config.Storage.Postgres.ConnectionURL, closeStorage: func() error { return adapter.Close(context.Background()) }}
	defer func() { _ = h.Close() }()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if err = h.start(&config, logger, nil, faults); err != nil {
		return errors.New("ledger child cannot build application")
	}
	encoder := json.NewEncoder(output)
	var outputMu sync.Mutex
	notify := func(message LedgerChildMessage) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		return encoder.Encode(message)
	}
	if err = notify(LedgerChildMessage{State: "ready", AdminURL: h.Admin.BaseURL(), EndUserURL: h.EndUser.BaseURL()}); err != nil {
		return err
	}
	var release chan struct{}
	defer func() {
		if release != nil {
			close(release)
		}
	}()
	for {
		var command string
		if err = decoder.Decode(&command); errors.Is(err, io.EOF) {
			// Closing stdin during a kill must not release the committed request.
			if release != nil {
				<-release
			}
			return nil
		} else if err != nil {
			return errors.New("invalid ledger child command")
		}
		switch command {
		case "hold_commit":
			if release != nil {
				return errors.New("ledger child commit already armed")
			}
			release = make(chan struct{})
			reached := faults.HoldNextCommit(release)
			go func() { <-reached; _ = notify(LedgerChildMessage{State: "committed"}) }()
			if err = notify(LedgerChildMessage{State: "armed"}); err != nil {
				return err
			}
		case "release_commit":
			if release == nil {
				return errors.New("ledger child commit is not armed")
			}
			close(release)
			release = nil
		case "shutdown":
			return nil
		default:
			return errors.New("unknown ledger child command")
		}
	}
}
