package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	brokerconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/spf13/cobra"
)

type CredentialServerOptions struct {
	Directory   string
	Document    map[string]any
	Environment map[string]string
	Command     *cobra.Command
	Principal   string
}

type CredentialServer struct {
	AdminURL   string
	EndUserURL string
	ConfigPath string
	Principal  string
	Logs       *BufferedLogCapture
	admin      *TestServer
	endUser    *TestServer
	storage    *storageadapter.Adapter
	logger     *slog.Logger
	client     *http.Client
}

func StartCredentialServer(options CredentialServerOptions) (*CredentialServer, error) {
	server, err := prepareCredentialServer(options)
	if err != nil {
		return nil, err
	}
	cfg, err := LoadCredentialConfiguration(server.ConfigPath, options.Environment, options.Command)
	if err != nil {
		server.logger.Error("failed to load configuration", "error", err)
		return server, err
	}
	return server, server.start(cfg)
}

func prepareCredentialServer(options CredentialServerOptions) (*CredentialServer, error) {
	endUserPort, err := credentialServerPort()
	if err != nil {
		return nil, err
	}
	adminPort, err := credentialServerPort()
	if err != nil {
		return nil, err
	}
	for adminPort == endUserPort {
		adminPort, err = credentialServerPort()
		if err != nil {
			return nil, err
		}
	}
	logger, logs := NewBufferedJSONLogger(slog.LevelInfo)
	server := &CredentialServer{
		AdminURL:   "http://127.0.0.1:" + strconv.Itoa(adminPort),
		EndUserURL: "http://127.0.0.1:" + strconv.Itoa(endUserPort),
		ConfigPath: filepath.Join(options.Directory, "credential-config.yaml"),
		Principal:  options.Principal,
		Logs:       logs,
		logger:     logger,
		client:     newTestHTTPClient(),
	}
	instances, ok := options.Document["server"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("credential fixture requires server configuration")
	}
	for name, values := range map[string]struct {
		port      int
		publicURL string
	}{
		"enduser": {endUserPort, server.EndUserURL},
		"admin":   {adminPort, server.AdminURL},
	} {
		instance, ok := instances[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("credential fixture requires %s server configuration", name)
		}
		instance["port"] = values.port
		instance["public_url"] = values.publicURL
	}
	encoded, err := json.MarshalIndent(options.Document, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(options.Directory, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(server.ConfigPath, encoded, 0o644); err != nil {
		return nil, err
	}
	return server, nil
}

func (s *CredentialServer) start(cfg *ports.Config) error {
	var err error
	s.storage, err = NewStorageFactory(s.logger).NewTestStorage()
	if err != nil {
		return err
	}
	application, err := NewServerFactory(cfg, s.logger).BuildApp(s.storage)
	if err != nil {
		return err
	}
	s.admin, err = NewTestServerV2(application, s.logger, WithServerType(ServerTypeAdmin), WithFixedPort(cfg.Server.Admin.Port))
	if err != nil {
		if application.Shutdown != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if shutdownErr := application.Shutdown(ctx); shutdownErr != nil {
				s.logger.Error("credential test application shutdown failed", "error", shutdownErr)
			}
		}
		return err
	}
	s.endUser, err = NewTestServerV2(application, s.logger, WithServerType(ServerTypeEndUser), WithFixedPort(cfg.Server.EndUser.Port))
	return err
}

func credentialServerPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	return port, listener.Close()
}

func LoadCredentialConfiguration(path string, environment map[string]string, command *cobra.Command) (*ports.Config, error) {
	saved := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "IDENTITY_BROKER_") {
			saved[key] = value
		}
	}
	defer func() {
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			if strings.HasPrefix(key, "IDENTITY_BROKER_") {
				_ = os.Unsetenv(key)
			}
		}
		for key, value := range saved {
			_ = os.Setenv(key, value)
		}
	}()
	for key := range saved {
		if err := os.Unsetenv(key); err != nil {
			return nil, err
		}
	}
	if err := os.Setenv("IDENTITY_BROKER_CONFIG_PATH", path); err != nil {
		return nil, err
	}
	for key, value := range environment {
		if err := os.Setenv(key, value); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loader := brokerconfig.NewLoader()
	loader.SetCommand(command)
	return loader.GetConfig(ctx)
}

func (s *CredentialServer) AdminJSON(method, path string, body any) (*http.Response, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return s.request(s.AdminURL, method, path, "application/json", bytes.NewReader(encoded))
}

func (s *CredentialServer) EndUserRequest(method, path string, body io.Reader) (*http.Response, error) {
	return s.request(s.EndUserURL, method, path, "application/x-www-form-urlencoded", body)
}

func (s *CredentialServer) request(baseURL, method, path, contentType string, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Remote-User", s.Principal)
	if body != nil {
		request.Header.Set("Content-Type", contentType)
	}
	return s.client.Do(request)
}

func (s *CredentialServer) Close() {
	if s.endUser != nil {
		s.endUser.Close()
		s.endUser = nil
	}
	if s.admin != nil {
		s.admin.Close()
		s.admin = nil
	}
	if s.storage != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.storage.Close(ctx); err != nil {
			s.logger.Error("credential test storage shutdown failed", "error", err)
		}
		s.storage = nil
	}
}
