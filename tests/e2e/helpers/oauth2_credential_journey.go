package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

type OAuth2CredentialJourney struct {
	Directory   string
	ConfigPath  string
	Principal   string
	Upstream    *MockUpstreamOAuth2Server
	Storage     *storageadapter.Adapter
	Application *app.App
	Config      *ports.Config
	Admin       *bootstrap.TestServer
	EndUser     *bootstrap.TestServer
	Logger      *slog.Logger
	Logs        *bootstrap.BufferedLogCapture
}

func NewOAuth2CredentialJourney() (*OAuth2CredentialJourney, error) {
	directory, err := os.MkdirTemp("", "broker-credential-journey-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	logger, logs := bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
	return &OAuth2CredentialJourney{
		Directory:  directory,
		ConfigPath: filepath.Join(directory, "config.yaml"),
		Principal:  fixtures.DefaultPrincipal().String(),
		Upstream:   NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse(),
		Logger:     logger,
		Logs:       logs,
	}, nil
}

func (j *OAuth2CredentialJourney) WritePair(clientID, clientSecret string) (map[string]string, error) {
	directory, err := os.MkdirTemp(j.Directory, "unrelated-pair-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		return nil, err
	}
	binding := map[string]string{
		"client_id_file":     filepath.Join(directory, "identity"),
		"client_secret_file": filepath.Join(directory, "credential"),
	}
	if err := PublishCredentialPair(binding, clientID, clientSecret); err != nil {
		return nil, err
	}
	for name, target := range map[string]string{"identity": "..data/identity", "credential": "..data/credential"} {
		if err := os.Symlink(target, filepath.Join(directory, name)); err != nil {
			return nil, err
		}
	}
	return binding, nil
}

func PublishCredentialPair(binding map[string]string, clientID, clientSecret string) error {
	directory := filepath.Dir(binding["client_id_file"])
	generation, err := os.MkdirTemp(directory, "generation-")
	if err != nil {
		return err
	}
	if err := os.Chmod(generation, 0o755); err != nil {
		return err
	}
	for name, value := range map[string]string{"identity": clientID, "credential": clientSecret} {
		if err := os.WriteFile(filepath.Join(generation, name), []byte(value), 0o644); err != nil {
			return err
		}
	}
	next := filepath.Join(directory, "..data.next")
	if err := os.Symlink(filepath.Base(generation), next); err != nil {
		return err
	}
	return os.Rename(next, filepath.Join(directory, "..data"))
}

func (j *OAuth2CredentialJourney) Start(bindings map[string]map[string]string) error {
	if bindings == nil {
		bindings = map[string]map[string]string{}
	}
	return j.StartDocument(fixtures.CredentialConfigurationDocument(j.Upstream.URL(), bindings, 8000, 14000))
}

func (j *OAuth2CredentialJourney) StartDocument(document map[string]any) error {
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(j.ConfigPath, encoded, 0o644); err != nil {
		return err
	}
	cfg, err := bootstrap.LoadCredentialConfiguration(j.ConfigPath, nil, nil)
	if err != nil {
		return err
	}
	j.Config = cfg
	j.Storage, err = bootstrap.NewStorageFactory(j.Logger).NewTestStorage()
	if err != nil {
		return err
	}
	j.Application, err = bootstrap.NewServerFactory(cfg, j.Logger).BuildApp(j.Storage)
	if err != nil {
		return err
	}
	j.Admin, err = bootstrap.NewAdminTestServer(j.Application, j.Logger)
	if err != nil {
		return err
	}
	j.EndUser, err = bootstrap.NewEndUserTestServer(j.Application, j.Logger)
	return err
}

func (j *OAuth2CredentialJourney) AdminJSON(method, path string, body any) (*http.Response, error) {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	return j.Admin.DirectRequest(method, path, j.Principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
}

func CredentialResponseObject(response *http.Response) (map[string]any, error) {
	defer func() { _ = response.Body.Close() }()
	var body map[string]any
	err := json.NewDecoder(response.Body).Decode(&body)
	return body, err
}

func (j *OAuth2CredentialJourney) Register(request map[string]any) (id.ServiceID, map[string]any, error) {
	response, err := j.AdminJSON(http.MethodPost, "/api/services", request)
	if err != nil {
		return id.ServiceID{}, nil, err
	}
	body, err := CredentialResponseObject(response)
	if err != nil {
		return id.ServiceID{}, nil, err
	}
	if response.StatusCode != http.StatusCreated {
		return id.ServiceID{}, body, fmt.Errorf("service registration returned HTTP %d", response.StatusCode)
	}
	value, _ := body["id"].(string)
	serviceID, err := id.ParseServiceID(value)
	return serviceID, body, err
}

func (j *OAuth2CredentialJourney) Authorize(serviceID id.ServiceID) (*http.Response, error) {
	return j.EndUser.AuthenticatedGET("/api/third-party/"+serviceID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(j.Config.Server.EndUser.PublicURL+"/done"), j.Principal)
}

func ProviderAuthorizationCallback(authorizationURL string) (*url.URL, error) {
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(authorizationURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("provider authorization returned HTTP %d", response.StatusCode)
	}
	return url.Parse(response.Header.Get("Location"))
}

func (j *OAuth2CredentialJourney) Complete(serviceID id.ServiceID) (*http.Response, error) {
	response, err := j.Authorize(serviceID)
	if err != nil {
		return nil, err
	}
	location := response.Header.Get("Location")
	_ = response.Body.Close()
	if response.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("authorization initiation returned HTTP %d", response.StatusCode)
	}
	callback, err := ProviderAuthorizationCallback(location)
	if err != nil {
		return nil, err
	}
	return j.EndUser.AuthenticatedGET(callback.RequestURI(), j.Principal)
}

func (j *OAuth2CredentialJourney) Refresh(serviceID id.ServiceID) (*http.Response, error) {
	return j.EndUser.AuthenticatedPOST("/api/third-party/"+serviceID.String()+"/session/refresh", j.Principal, "application/json", nil)
}

func CredentialTokenRequest(request CapturedTokenRequest) (string, string, error) {
	form, err := url.ParseQuery(request.Body)
	if err != nil {
		return "", "", err
	}
	httpRequest := &http.Request{Header: request.Header}
	clientID, clientSecret, basic := httpRequest.BasicAuth()
	if !basic {
		return form.Get("client_id"), form.Get("client_secret"), nil
	}
	clientID, err = url.QueryUnescape(clientID)
	if err != nil {
		return "", "", err
	}
	clientSecret, err = url.QueryUnescape(clientSecret)
	return clientID, clientSecret, err
}

func (j *OAuth2CredentialJourney) Close() error {
	var shutdownErr, storageErr error
	if j.EndUser != nil {
		j.EndUser.Close()
	}
	if j.Admin != nil {
		j.Admin.Close()
	}
	if j.Application != nil && j.Application.Shutdown != nil {
		shutdownErr = j.Application.Shutdown(context.Background())
	}
	if j.Storage != nil {
		storageErr = j.Storage.Close(context.Background())
	}
	if j.Upstream != nil {
		j.Upstream.Close()
	}
	return errors.Join(shutdownErr, storageErr, os.RemoveAll(j.Directory))
}
