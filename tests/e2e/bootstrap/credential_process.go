package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type CredentialProcessOptions struct {
	Directory   string
	Document    map[string]any
	Environment map[string]string
	Principal   string
	NonRoot     bool
}

type CredentialProcess struct {
	*CredentialServer
	UID     uint32
	cmd     *exec.Cmd
	done    chan struct{}
	exitErr error
	cancel  context.CancelFunc
}

// StartCredentialProcess isolates privilege and blocking-file tests in the current test executable.
func StartCredentialProcess(options CredentialProcessOptions) (*CredentialProcess, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	server, err := prepareCredentialServer(CredentialServerOptions{
		Directory: options.Directory,
		Document:  options.Document,
		Principal: options.Principal,
	})
	if err != nil {
		return nil, err
	}
	uid := os.Geteuid()
	if uid < 0 || uid > math.MaxUint32 {
		return nil, fmt.Errorf("broker UID is outside the supported range")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	process := &CredentialProcess{
		CredentialServer: server,
		UID:              uint32(uid),
		done:             make(chan struct{}),
		cancel:           cancel,
		cmd:              exec.CommandContext(ctx, binary, "-test.run=^TestCredentialBrokerProcess$", "-test.count=1", "-test.timeout=35s"),
	}
	if options.NonRoot {
		if err := process.dropRootPrivileges(options.Directory); err != nil {
			cancel()
			return nil, err
		}
	}
	process.cmd.Dir = options.Directory
	process.cmd.Env = credentialProcessEnvironment(process.ConfigPath, options.Environment)
	process.cmd.Env = append(process.cmd.Env, "AIB_CREDENTIAL_BROKER_HELPER=1", "AIB_CREDENTIAL_BROKER_PRINCIPAL="+options.Principal)
	process.cmd.Stdout = process.Logs
	process.cmd.Stderr = process.Logs
	if err := process.cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	go func() {
		process.exitErr = process.cmd.Wait()
		close(process.done)
	}()
	if err := process.waitReady(); err != nil {
		return process, err
	}
	return process, nil
}

// RunCredentialBrokerProcess is called only by the isolated E2E test helper.
func RunCredentialBrokerProcess() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	path := os.Getenv("IDENTITY_BROKER_CONFIG_PATH")
	loaderEnvironment := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "IDENTITY_BROKER_") && key != "IDENTITY_BROKER_CONFIG_PATH" {
			loaderEnvironment[key] = value
		}
	}
	cfg, err := LoadCredentialConfiguration(path, loaderEnvironment, nil)
	if err != nil {
		return err
	}
	server := &CredentialServer{
		AdminURL:   cfg.Server.Admin.PublicURL,
		EndUserURL: cfg.Server.EndUser.PublicURL,
		ConfigPath: path,
		Principal:  os.Getenv("AIB_CREDENTIAL_BROKER_PRINCIPAL"),
		logger:     slog.New(slog.NewJSONHandler(os.Stdout, nil)),
		client:     newTestHTTPClient(),
	}
	slog.SetDefault(server.logger)
	defer server.Close()
	if err := server.start(cfg); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

func (p *CredentialProcess) dropRootPrivileges(directory string) error {
	if p.UID != 0 {
		return nil
	}
	nobody, err := user.Lookup("nobody")
	if err != nil {
		return err
	}
	uid, err := strconv.ParseUint(nobody.Uid, 10, 64)
	if err != nil {
		return err
	}
	gid, err := strconv.ParseUint(nobody.Gid, 10, 64)
	if err != nil {
		return err
	}
	if uid > math.MaxUint32 || gid > math.MaxUint32 {
		return fmt.Errorf("non-root UID or GID is outside the supported range")
	}
	if uid == 0 {
		return fmt.Errorf("non-root broker UID is required")
	}
	copiedDirectory, err := os.MkdirTemp(directory, "broker-test-process-")
	if err != nil {
		return err
	}
	if err := os.Chmod(copiedDirectory, 0o755); err != nil {
		return err
	}
	copiedBinary := filepath.Join(copiedDirectory, "broker-test-process")
	input, err := os.Open(p.cmd.Path)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(copiedBinary, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(copiedBinary, 0o755); err != nil {
		return err
	}
	p.UID = uint32(uid)
	p.cmd.Path = copiedBinary
	p.cmd.Args[0] = copiedBinary
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}}
	return nil
}

func credentialProcessEnvironment(configPath string, overrides map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(overrides)+1)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "IDENTITY_BROKER_") {
			environment = append(environment, entry)
		}
	}
	environment = append(environment, "IDENTITY_BROKER_CONFIG_PATH="+configPath)
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func (p *CredentialProcess) waitReady() error {
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return fmt.Errorf("broker test process exited before readiness: %v; output: %s", p.exitErr, p.Logs.Raw())
		case <-deadline.C:
			return fmt.Errorf("broker test process readiness deadline exceeded; output: %s", p.Logs.Raw())
		case <-ticker.C:
			ready := true
			for _, baseURL := range []string{p.AdminURL, p.EndUserURL} {
				response, err := p.client.Get(baseURL + "/health")
				if err != nil {
					ready = false
					break
				}
				_ = response.Body.Close()
				if response.StatusCode != http.StatusOK {
					ready = false
					break
				}
			}
			if ready {
				return nil
			}
		}
	}
}

func (p *CredentialProcess) Close() {
	defer p.cancel()
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		p.cancel()
		<-p.done
	}
}
