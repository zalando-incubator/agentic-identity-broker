package oauth2session_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/credentialfile"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type credentialReaderSpy struct {
	reader     ports.CredentialFileReader
	mu         sync.Mutex
	ids        int
	pairs      int
	beforePair func(int) error
	afterPair  func() error
}

func (r *credentialReaderSpy) ReadClientID(path string) (string, error) {
	r.mu.Lock()
	r.ids++
	r.mu.Unlock()
	return r.reader.ReadClientID(path)
}

func (r *credentialReaderSpy) ReadPair(clientIDPath, secretPath string) (string, string, error) {
	r.mu.Lock()
	r.pairs++
	attempt := r.pairs
	r.mu.Unlock()
	if r.beforePair != nil {
		if err := r.beforePair(attempt); err != nil {
			return "", "", err
		}
	}
	clientID, secret, err := r.reader.ReadPair(clientIDPath, secretPath)
	if err == nil && r.afterPair != nil {
		err = r.afterPair()
	}
	if err != nil {
		return "", "", err
	}
	return clientID, secret, nil
}

func (r *credentialReaderSpy) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ids, r.pairs
}

type credentialTokenRequest struct {
	clientID string
	secret   string
	form     url.Values
	header   http.Header
}

type credentialTokenEndpoint struct {
	server       *httptest.Server
	mu           sync.Mutex
	requests     []credentialTokenRequest
	errors       []error
	onRequest    func(credentialTokenRequest) (int, error)
	responseBody string
}

func newCredentialTokenEndpoint(t *testing.T) *credentialTokenEndpoint {
	t.Helper()
	endpoint := &credentialTokenEndpoint{}
	endpoint.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			endpoint.mu.Lock()
			endpoint.errors = append(endpoint.errors, err)
			endpoint.mu.Unlock()
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		clientID, secret := request.Form.Get("client_id"), request.Form.Get("client_secret")
		if basicID, basicSecret, ok := request.BasicAuth(); ok {
			clientID, _ = url.QueryUnescape(basicID)
			secret, _ = url.QueryUnescape(basicSecret)
		}
		captured := credentialTokenRequest{clientID: clientID, secret: secret, form: request.Form, header: request.Header.Clone()}
		endpoint.mu.Lock()
		endpoint.requests = append(endpoint.requests, captured)
		endpoint.mu.Unlock()
		status := http.StatusOK
		if endpoint.onRequest != nil {
			var err error
			status, err = endpoint.onRequest(captured)
			if err != nil {
				endpoint.mu.Lock()
				endpoint.errors = append(endpoint.errors, err)
				endpoint.mu.Unlock()
				status = http.StatusInternalServerError
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if endpoint.responseBody != "" {
			_, _ = w.Write([]byte(endpoint.responseBody))
			return
		}
		if status != http.StatusOK {
			_, _ = w.Write([]byte(`{"error":"server_error"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"credential-access","refresh_token":"credential-refresh","token_type":"Bearer","expires_in":3600,"scope":"repo user"}`))
	}))
	t.Cleanup(endpoint.server.Close)
	return endpoint
}

func (e *credentialTokenEndpoint) captured(t *testing.T) []credentialTokenRequest {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	require.Empty(t, e.errors, "synthetic provider setup must not fail")
	return append([]credentialTokenRequest(nil), e.requests...)
}

type credentialSessionHarness struct {
	service   *oauth2session.OAuth2SessionService
	providers *thirdparty.ThirdpartyOAuth2ProviderService
	records   *memory.InMemoryThirdpartyOAuth2ProviderRepository
	sessions  ports.UserSessionRepository
	reader    *credentialReaderSpy
	endpoint  *credentialTokenEndpoint
}

func newCredentialSessionHarness(t *testing.T, bindings map[string]ports.CredentialFileBinding, loggers ...*slog.Logger) *credentialSessionHarness {
	t.Helper()
	service, records, sessions, _, _, providers := setupServiceWithConfig(t, func(config *oauth2session.Config) {
		config.CredentialFiles = bindings
		config.MaxRetries = 3
		config.RetryBaseDelay = time.Millisecond
	}, loggers...)
	reader := &credentialReaderSpy{reader: credentialfile.New()}
	service.WithCredentialFileReader(reader)
	return &credentialSessionHarness{service: service, providers: providers, records: records, sessions: sessions, reader: reader, endpoint: newCredentialTokenEndpoint(t)}
}

func credentialPair(t *testing.T, clientID, secret string) ports.CredentialFileBinding {
	t.Helper()
	directory := t.TempDir()
	binding := ports.CredentialFileBinding{ClientIDFile: filepath.Join(directory, "identity-input"), ClientSecretFile: filepath.Join(directory, "authentication-input")}
	require.NoError(t, os.WriteFile(binding.ClientIDFile, []byte(clientID), 0600))
	require.NoError(t, os.WriteFile(binding.ClientSecretFile, []byte(secret), 0600))
	return binding
}

func publishCredential(path, value string) error {
	file, err := os.CreateTemp(filepath.Dir(path), "publication-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.WriteString(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (h *credentialSessionHarness) register(t *testing.T, source model.CredentialSource, canonicalID, clientID, secret string) *model.ThirdpartyOAuth2ProviderEntity {
	t.Helper()
	provider := createTestService(id.NewServiceID())
	provider.DisplayName = "display-label"
	provider.CanonicalID = &canonicalID
	provider.CredentialSource = source
	provider.ClientID = id.ClientID(clientID)
	provider.Secret = model.NewPlaintextSecret(secret)
	if source == model.CredentialSourceFilesystem {
		provider.Secret = model.NewAbsentSecret()
	}
	provider.Endpoints.TokenEndpoint = h.endpoint.server.URL
	require.NoError(t, h.providers.Create(context.Background(), provider))
	return provider
}

func (h *credentialSessionHarness) initiate(t *testing.T, principal id.Principal, serviceID id.ServiceID) *oauth2session.InitiateFlowResult {
	t.Helper()
	flow, err := h.service.InitiateOAuth2Flow(context.Background(), principal, serviceID, "https://broker.example.com/sessions")
	require.NoError(t, err)
	require.NotNil(t, flow)
	return flow
}

func (h *credentialSessionHarness) callback(t *testing.T, principal id.Principal, serviceID id.ServiceID, state string) *storage.UserSession {
	t.Helper()
	result, err := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: serviceID, Code: "credential-code", State: state})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Session)
	return result.Session
}

func (h *credentialSessionHarness) expire(t *testing.T, principal id.Principal, serviceID id.ServiceID) {
	t.Helper()
	session, err := h.sessions.FindByPrincipalAndService(context.Background(), principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, session)
	updated := *session
	expired := time.Now().Add(-time.Minute)
	updated.AccessTokenExpiresAt = &expired
	require.NoError(t, h.sessions.Create(context.Background(), &updated))
}

func (h *credentialSessionHarness) seedSession(t *testing.T, principal id.Principal, serviceID id.ServiceID, established *id.ClientID) *storage.UserSession {
	t.Helper()
	ctx := context.Background()
	encryption := newTestEncryption(t)
	aad := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	access, err := encryption.Encrypt(ctx, []byte("legacy-access"), aad)
	require.NoError(t, err)
	refresh, err := encryption.Encrypt(ctx, []byte("legacy-refresh"), aad)
	require.NoError(t, err)
	now := time.Now()
	expired := now.Add(-time.Minute)
	session := &storage.UserSession{ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID, UpstreamClientID: established, EncryptedAccessToken: access, EncryptedRefreshToken: refresh, TokenType: "Bearer", AccessTokenExpiresAt: &expired, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID}, CreatedAt: now, InitiatedAt: now, UpdatedAt: now}
	require.NoError(t, h.sessions.Create(ctx, session))
	return session
}

func assertCredentialSourceReason(t *testing.T, err error, reason model.CredentialSourceReason) {
	t.Helper()
	require.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
	var sourceError *model.CredentialSourceError
	require.ErrorAs(t, err, &sourceError)
	assert.Equal(t, reason, sourceError.Reason)
}

func TestCredentialSelection_StoredIgnoresBindingsThroughoutConnectionAndRefresh(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty mapping", true: "matching unavailable binding"}[configured], func(t *testing.T) {
			bindings := map[string]ports.CredentialFileBinding{}
			if configured {
				bindings["stored-service"] = ports.CredentialFileBinding{ClientIDFile: filepath.Join(t.TempDir(), "absent-id"), ClientSecretFile: filepath.Join(t.TempDir(), "absent-secret")}
			}
			h := newCredentialSessionHarness(t, bindings)
			provider := h.register(t, model.CredentialSourceStored, "stored-service", "stored-client", "stored-secret")
			before, err := h.records.Get(context.Background(), provider.ID)
			require.NoError(t, err)
			principal := id.Principal("stored@example.com")
			flow := h.initiate(t, principal, provider.ID)
			claims, err := h.service.ValidateStateToken(flow.StateToken, principal, provider.ID)
			require.NoError(t, err)
			require.NotNil(t, claims.UpstreamClientID)
			assert.Equal(t, id.ClientID("stored-client"), *claims.UpstreamClientID)
			session := h.callback(t, principal, provider.ID, flow.StateToken)
			assert.Equal(t, claims.UpstreamClientID, session.UpstreamClientID)
			_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
			require.NoError(t, err)
			h.expire(t, principal, provider.ID)
			refreshed, token, err := h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, "credential-access", token)
			assert.Equal(t, claims.UpstreamClientID, refreshed.UpstreamClientID)
			requests := h.endpoint.captured(t)
			require.Len(t, requests, 3)
			for _, request := range requests {
				assert.Equal(t, "stored-client", request.clientID)
				assert.Equal(t, "stored-secret", request.secret)
			}
			ids, pairs := h.reader.counts()
			assert.Zero(t, ids)
			assert.Zero(t, pairs)
			after, err := h.records.Get(context.Background(), provider.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestCredentialSelection_FilesystemUsesExactPairAndPersistsOnlyVerifiedSessionIdentity(t *testing.T) {
	binding := credentialPair(t, " \tfile client\n", "\nfile secret\t ")
	decoy := credentialPair(t, "decoy-client", "decoy-secret")
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"canonical-service": binding, "display-label": decoy, "client-label": decoy})
	provider := h.register(t, model.CredentialSourceFilesystem, "canonical-service", "", "")
	before, err := h.records.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	principal := id.Principal("filesystem@example.com")
	flow := h.initiate(t, principal, provider.ID)
	authorization, err := url.Parse(flow.AuthorizationURL)
	require.NoError(t, err)
	assert.Equal(t, "file client", authorization.Query().Get("client_id"))
	claims, err := h.service.ValidateStateToken(flow.StateToken, principal, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, claims.UpstreamClientID)
	assert.Equal(t, id.ClientID("file client"), *claims.UpstreamClientID)
	ids, pairs := h.reader.counts()
	assert.Equal(t, 1, ids)
	assert.Zero(t, pairs)
	session := h.callback(t, principal, provider.ID, flow.StateToken)
	assert.Equal(t, claims.UpstreamClientID, session.UpstreamClientID)
	persisted, err := h.sessions.Get(context.Background(), session.ID)
	require.NoError(t, err)
	assert.Equal(t, claims.UpstreamClientID, persisted.UpstreamClientID)
	_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	h.expire(t, principal, provider.ID)
	refreshed, _, err := h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, claims.UpstreamClientID, refreshed.UpstreamClientID)
	requests := h.endpoint.captured(t)
	require.Len(t, requests, 3)
	for _, request := range requests {
		assert.Equal(t, "file client", request.clientID)
		assert.Equal(t, "file secret", request.secret)
	}
	ids, pairs = h.reader.counts()
	assert.Equal(t, 1, ids)
	assert.Equal(t, 3, pairs)
	after, err := h.records.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.True(t, after.ClientID.IsZero())
	assert.True(t, after.Secret.IsAbsent())
	metadata, err := h.service.ListUserSessions(context.Background(), principal)
	require.NoError(t, err)
	encoded, err := json.Marshal(metadata)
	require.NoError(t, err)
	for _, value := range []string{"file client", "file secret", binding.ClientIDFile, binding.ClientSecretFile, "upstream_client_id"} {
		assert.NotContains(t, string(encoded), value)
	}
	encoded, err = json.Marshal(refreshed)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "upstream_client_id")
	assert.NotContains(t, string(encoded), "file client")
	newIDs, newPairs := h.reader.counts()
	assert.Equal(t, ids, newIDs)
	assert.Equal(t, pairs, newPairs)
}

func TestCredentialSelection_InitiationDoesNotRequireOrReadSecretFile(t *testing.T) {
	binding := credentialPair(t, "id-without-secret", "unused-secret")
	require.NoError(t, os.Remove(binding.ClientSecretFile))
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"id-only-service": binding})
	provider := h.register(t, model.CredentialSourceFilesystem, "id-only-service", "", "")
	principal := id.Principal("id-only@example.com")
	flow := h.initiate(t, principal, provider.ID)
	ids, pairs := h.reader.counts()
	assert.Equal(t, 1, ids)
	assert.Zero(t, pairs)
	_, err := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "credential-code", State: flow.StateToken})
	assertCredentialSourceReason(t, err, model.CredentialSourceReasonNotFound)
	assert.Empty(t, h.endpoint.captured(t))
	ids, pairs = h.reader.counts()
	assert.Equal(t, 1, ids)
	assert.Equal(t, 1, pairs)
}

func TestCredentialSelection_MissingExactBindingFailsClosedAtEveryBoundary(t *testing.T) {
	for _, bindings := range []string{"empty", "different canonical ID", "case variant"} {
		for _, operation := range []string{"initiation", "callback", "explicit refresh", "automatic refresh"} {
			t.Run(bindings+"/"+operation, func(t *testing.T) {
				mapping := map[string]ports.CredentialFileBinding{}
				if bindings != "empty" {
					mapping["other-service"] = credentialPair(t, "other-client", "other-secret")
				}
				if bindings == "case variant" {
					mapping["Unbound-service"] = credentialPair(t, "case-variant-client", "case-variant-secret")
				}
				h := newCredentialSessionHarness(t, mapping)
				provider := h.register(t, model.CredentialSourceFilesystem, "unbound-service", "", "")
				principal := id.Principal("unbound@example.com")
				established := id.ClientID("established-client")
				var err error
				switch operation {
				case "initiation":
					_, err = h.service.InitiateOAuth2Flow(context.Background(), principal, provider.ID, "https://broker.example.com/sessions")
				case "callback":
					state := credentialState(t, h.service, principal, provider.ID, &established)
					_, err = h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "credential-code", State: state})
				default:
					h.seedSession(t, principal, provider.ID, &established)
					if operation == "explicit refresh" {
						_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
					} else {
						_, _, err = h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
					}
				}
				assertCredentialSourceReason(t, err, model.CredentialSourceReasonMissingBinding)
				assert.Empty(t, h.endpoint.captured(t))
				ids, pairs := h.reader.counts()
				assert.Zero(t, ids)
				assert.Zero(t, pairs)
			})
		}
	}
}
