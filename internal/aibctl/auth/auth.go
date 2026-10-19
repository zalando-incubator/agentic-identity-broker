// Package auth produces the credentials attached to admin API requests: an
// optional Bearer token for the reverse proxy and an optional principal header
// for the broker.
package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Error marks failures to obtain credentials; callers map it to exit code 3.
type Error struct {
	Msg string
	Err error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type StaticToken string

func (s StaticToken) Token(context.Context) (string, error) { return string(s), nil }

// Runner executes argv and returns its stdout and stderr.
type Runner func(ctx context.Context, argv []string) (stdout, stderr []byte, err error)

// CommandToken runs an external program and uses its trimmed stdout as the token.
type CommandToken struct {
	Argv    []string
	Timeout time.Duration
	Runner  Runner
}

const maxStderrInError = 4096

func (c CommandToken) Token(ctx context.Context) (string, error) {
	if len(c.Argv) == 0 {
		return "", &Error{Msg: "token_command is empty"}
	}
	runner := c.Runner
	if runner == nil {
		runner = defaultRunner
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	name := strings.Join(c.Argv, " ")
	stdout, stderr, err := runner(ctx, c.Argv)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", &Error{Msg: fmt.Sprintf("token command %q timed out after %s", name, c.Timeout)}
	}
	if err != nil {
		detail := strings.TrimSpace(string(tail(stderr, maxStderrInError)))
		if detail != "" {
			return "", &Error{Msg: fmt.Sprintf("token command %q failed (%v): %s", name, err, detail)}
		}
		return "", &Error{Msg: fmt.Sprintf("token command %q failed", name), Err: err}
	}
	token := strings.TrimSpace(string(stdout))
	if token == "" {
		return "", &Error{Msg: fmt.Sprintf("token command %q produced no output", name)}
	}
	return token, nil
}

func defaultRunner(ctx context.Context, argv []string) ([]byte, []byte, error) {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.CommandContext(ctx, path, argv[1:]...) // #nosec G204 -- argv comes from the user's own config.
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func tail(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}

// Authenticator applies credentials to outgoing requests. The token is
// resolved at most once per process.
type Authenticator struct {
	source          TokenSource
	principalHeader string
	principal       string

	once  sync.Once
	token string
	err   error
}

func New(source TokenSource, principalHeader, principal string) *Authenticator {
	return &Authenticator{source: source, principalHeader: principalHeader, principal: principal}
}

func (a *Authenticator) Apply(ctx context.Context, req *http.Request) error {
	if a.source != nil {
		a.once.Do(func() { a.token, a.err = a.source.Token(ctx) })
		if a.err != nil {
			return a.err
		}
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	if a.principal != "" && a.principalHeader != "" {
		req.Header.Set(a.principalHeader, a.principal)
	}
	return nil
}
