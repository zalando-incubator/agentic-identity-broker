package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Keep the real mutation semantics while observing only the existing port boundary.
type identityBoundaryRepository struct {
	*memory.InMemoryThirdpartyOAuth2ProviderRepository
	afterGet        func(context.Context, *model.ThirdpartyOAuth2ProviderEntity)
	beforeWrite     func(*model.ThirdpartyOAuth2ProviderEntity)
	afterUpdate     func(bool, error)
	updateError     error
	identityChanged bool
}

func (r *identityBoundaryRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	entity, err := r.InMemoryThirdpartyOAuth2ProviderRepository.Get(ctx, serviceID)
	if err == nil && r.afterGet != nil {
		r.afterGet(ctx, entity)
	}
	return entity, err
}

func (r *identityBoundaryRepository) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	if r.beforeWrite != nil {
		r.beforeWrite(entity)
	}
	return r.InMemoryThirdpartyOAuth2ProviderRepository.Create(ctx, entity)
}

func (r *identityBoundaryRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expected *int64) (bool, error) {
	if r.beforeWrite != nil {
		r.beforeWrite(entity)
	}
	changed, err := false, r.updateError
	if err == nil {
		changed, err = r.InMemoryThirdpartyOAuth2ProviderRepository.Update(ctx, entity, expected)
	}
	r.identityChanged = changed
	if r.afterUpdate != nil {
		r.afterUpdate(changed, err)
	}
	return changed, err
}

// Observe the type before the production policy renders it for the JSON sink.
type identityPrincipalTypeHandler struct {
	slog.Handler
	t         *testing.T
	operators *[]id.Principal
}

func (h identityPrincipalTypeHandler) Handle(ctx context.Context, record slog.Record) error {
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "operator_principal" {
			operator, ok := attr.Value.Any().(id.Principal)
			assert.True(h.t, ok, "operator attribution must reach the principal policy as id.Principal")
			assert.NotEqual(h.t, slog.LevelDebug, record.Level)
			*h.operators = append(*h.operators, operator)
		}
		return true
	})
	return h.Handler.Handle(ctx, record)
}

func identityAuditLogger(t *testing.T, logs *bytes.Buffer) (*slog.Logger, *[]id.Principal) {
	t.Helper()
	operators := new([]id.Principal)
	handler := telemetry.NewContextHandler(telemetry.NewPrincipalPolicyHandler(
		slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return slog.New(identityPrincipalTypeHandler{Handler: handler, t: t, operators: operators}), operators
}

func identityAuditEvents(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["event"] == "session.oauth2.client_identity_changed" {
			events = append(events, record)
		}
	}
	return events
}

func identityAuditRouter(t *testing.T, repo ports.ThirdpartyOAuth2ProviderRepository, config *ports.Config, logger *slog.Logger) (http.Handler, *thirdparty.ThirdpartyOAuth2ProviderService) {
	t.Helper()
	service := thirdparty.NewThirdpartyOAuth2ProviderService(repo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, logger)
	handler := NewServicesHandler(service, config, logger)
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(principal.WithPrincipal(r.Context(), id.Principal("operator@example.test"))))
		})
	})
	router.Post("/api/services", handler.CreateService)
	router.Put("/api/services/{service-id}", handler.UpdateService)
	router.Get("/api/services", handler.ListServices)
	router.Get("/api/services/{service-id}", handler.GetService)
	return router, service
}

func assertIdentityResponseHidden(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	for _, field := range []string{"client_identity", "client_identity_revision", "upstream_client_id", "upstream_client_revision", "ClientIdentity", "ClientIdentityRevision"} {
		assert.NotContains(t, response.Body.String(), `"`+field+`"`)
	}
}

func TestServicesHandler_ClientIdentityCommittedAdminAudit(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		invalid    bool
		stale      bool
		conflict   bool
		writeError bool
	}{
		{name: "accepted stored ID update", status: http.StatusOK},
		{name: "validation rejection", status: http.StatusBadRequest, invalid: true},
		{name: "version conflict", status: http.StatusPreconditionFailed, stale: true},
		{name: "resource ownership conflict", status: http.StatusConflict, conflict: true},
		{name: "failed commit", status: http.StatusInternalServerError, writeError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &identityBoundaryRepository{InMemoryThirdpartyOAuth2ProviderRepository: memory.NewInMemoryThirdpartyOAuth2ProviderRepository()}
			stored := credentialSourceHandlerStoredFixture(t, repo)
			before, err := repo.Get(t.Context(), stored.ID)
			require.NoError(t, err)
			body := credentialSourceHandlerBody()
			body["client_id"] = "private-replacement-client-ID"
			body["client_secret"] = "private-replacement-client-secret"
			body["protected_resources"] = before.ProtectedResources
			etag := strongETag(before.Version)
			if tc.invalid {
				body["display_name"] = ""
			}
			if tc.stale {
				etag = strongETag(before.Version + 1)
			}
			if tc.conflict {
				other := encryptedEntity(id.NewServiceID(), "Other service", "other-client", "other-secret", "https://source.example.com", nil)
				other.ProtectedResources = []string{"https://resource.example.com/already-owned"}
				require.NoError(t, repo.Create(t.Context(), other))
				body["protected_resources"] = other.ProtectedResources
			}
			if tc.writeError {
				repo.updateError = storage.NewStorageError("Update", storage.ErrorKindConnection, nil, "transaction commit failed")
			}
			var logs bytes.Buffer
			logger, typedOperators := identityAuditLogger(t, &logs)
			router, _ := identityAuditRouter(t, repo, testConfig(), logger)
			repo.afterUpdate = func(bool, error) {
				assert.Empty(t, identityAuditEvents(t, &logs), "the event must wait for the successful repository return")
			}

			response := credentialSourceHandlerRequest(t, router, http.MethodPut, "/api/services/"+stored.ID.String(), body, etag)

			require.Equal(t, tc.status, response.Code, response.Body.String())
			assertIdentityResponseHidden(t, response)
			after, err := repo.Get(t.Context(), stored.ID)
			require.NoError(t, err)
			events := identityAuditEvents(t, &logs)
			if tc.status == http.StatusOK {
				assert.True(t, repo.identityChanged)
				assert.Equal(t, before.ClientIdentityRevision+1, after.ClientIdentityRevision)
				require.NotNil(t, after.ClientIdentity)
				assert.Equal(t, id.ClientID("private-replacement-client-ID"), *after.ClientIdentity)
				assert.Equal(t, strongETag(after.Version), response.Header().Get("ETag"))
				require.Len(t, events, 1)
				assert.Equal(t, "INFO", events[0]["level"])
				assert.Equal(t, stored.ID.String(), events[0]["service_id"])
				assert.Equal(t, float64(after.ClientIdentityRevision), events[0]["client_identity_revision"])
				assert.Equal(t, "admin_update", events[0]["channel"])
				assert.Equal(t, "operator@example.test", events[0]["operator_principal"], "the production principal policy currently renders the identity")
				assert.Equal(t, []id.Principal{"operator@example.test"}, *typedOperators)
			} else {
				assert.False(t, repo.identityChanged)
				assert.Equal(t, before, after, "rejected and rolled-back updates must preserve all evidence and ciphertext")
				assert.Empty(t, events)
				assert.Empty(t, *typedOperators)
			}
			for _, sensitive := range []string{"stored-client", "stored-secret", "private-replacement-client-ID", "private-replacement-client-secret"} {
				assert.NotContains(t, logs.String(), sensitive)
			}
		})
	}
}

func TestServicesHandler_ClientIdentityInternalInputsAreUnboundAcrossCRUD(t *testing.T) {
	for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		t.Run(string(source), func(t *testing.T) {
			repo := &identityBoundaryRepository{InMemoryThirdpartyOAuth2ProviderRepository: memory.NewInMemoryThirdpartyOAuth2ProviderRepository()}
			writes := 0
			repo.beforeWrite = func(entity *model.ThirdpartyOAuth2ProviderEntity) {
				writes++
				assert.Nil(t, entity.ClientIdentity, "unknown identity fields must not bind, even before storage normalizes them")
				assert.Zero(t, entity.ClientIdentityRevision)
			}
			var logs bytes.Buffer
			logger, _ := identityAuditLogger(t, &logs)
			config := testConfig()
			config.ThirdPartyOAuth2.CredentialFiles = map[string]ports.CredentialFileBinding{
				"credential-source": {ClientIDFile: "/unavailable/private-id-basename", ClientSecretFile: "/unavailable/private-secret-basename"},
			}
			router, _ := identityAuditRouter(t, repo, config, logger)
			body := credentialSourceHandlerBody()
			body["credential_source"] = source
			if source == model.CredentialSourceStored {
				body["client_id"], body["client_secret"] = "public-configured-client", "private-configured-secret"
			}
			// These are deliberately invalid internal types/values. Unknown request
			// fields retain the existing ignore semantics; they are not a new API.
			body["client_identity"] = map[string]any{"value": "private-forged-identity"}
			body["client_identity_revision"] = -42
			body["upstream_client_revision"] = "private-forged-revision"
			body["ClientIdentity"] = "private-forged-go-field"
			body["ClientIdentityRevision"] = 9000
			created := credentialSourceHandlerRequest(t, router, http.MethodPost, "/api/services", body, "")
			require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
			assertIdentityResponseHidden(t, created)
			var result struct {
				ID id.ServiceID `json:"id"`
			}
			require.NoError(t, json.Unmarshal(created.Body.Bytes(), &result))
			body["display_name"] = "Renamed metadata"
			updated := credentialSourceHandlerRequest(t, router, http.MethodPut, "/api/services/"+result.ID.String(), body, "")
			require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
			assertIdentityResponseHidden(t, updated)
			for _, path := range []string{"/api/services/" + result.ID.String(), "/api/services/credential-source", "/api/services"} {
				response := credentialSourceHandlerRequest(t, router, http.MethodGet, path, nil, "")
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assertIdentityResponseHidden(t, response)
				assert.NotContains(t, response.Body.String(), "private-forged")
			}
			stored, err := repo.Get(t.Context(), result.ID)
			require.NoError(t, err)
			assert.Zero(t, stored.ClientIdentityRevision)
			if source == model.CredentialSourceFilesystem {
				assert.Nil(t, stored.ClientIdentity, "metadata CRUD must not resolve files or record their identity")
			} else {
				require.NotNil(t, stored.ClientIdentity)
				assert.Equal(t, id.ClientID("public-configured-client"), *stored.ClientIdentity)
			}
			assert.Equal(t, 2, writes)
			assert.False(t, repo.identityChanged)
			assert.Empty(t, identityAuditEvents(t, &logs), "creation, initialization and same-ID metadata updates record no change")
			for _, sensitive := range []string{"private-forged", "private-configured-secret", "private-id-basename", "private-secret-basename"} {
				assert.NotContains(t, logs.String(), sensitive)
				assert.NotContains(t, created.Body.String(), sensitive)
				assert.NotContains(t, updated.Body.String(), sensitive)
			}
		})
	}
}

type identityMetadataFileReader struct {
	identity  string
	idReads   int
	pairReads int
}

func (r *identityMetadataFileReader) ReadClientID(string) (string, error) {
	r.idReads++
	return r.identity, nil
}

func (r *identityMetadataFileReader) ReadPair(string, string) (string, string, error) {
	r.pairReads++
	return r.identity, "private-file-secret", nil
}

func TestServicesHandler_ClientIdentityPausedFilesystemPUTPreservesObservationAudit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	repo := &identityBoundaryRepository{InMemoryThirdpartyOAuth2ProviderRepository: memory.NewInMemoryThirdpartyOAuth2ProviderRepository()}
	stored := credentialSourceHandlerFilesystemFixture()
	require.NoError(t, repo.Create(ctx, stored))
	oldID := id.ClientID("private-file-client-before")
	_, applied, changed, err := repo.RecordObservedClientIdentity(ctx, stored.ID, oldID, ports.ClientIdentityObservation{
		ServiceVersion: stored.Version, CanonicalID: *stored.CanonicalID, Revision: 0,
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.False(t, changed)
	before, err := repo.Get(ctx, stored.ID)
	require.NoError(t, err)
	var logs bytes.Buffer
	logger, typedOperators := identityAuditLogger(t, &logs)
	binding := ports.CredentialFileBinding{
		ClientIDFile: filepath.Join(t.TempDir(), "private-client-id-basename"), ClientSecretFile: filepath.Join(t.TempDir(), "private-client-secret-basename"),
	}
	config := testConfig()
	config.ThirdPartyOAuth2.CredentialFiles = map[string]ports.CredentialFileBinding{*stored.CanonicalID: binding}
	router, provider := identityAuditRouter(t, repo, config, logger)
	reader := &identityMetadataFileReader{identity: "private-file-client-after"}
	sessions := oauth2session.NewOAuth2SessionService(provider, nil, nil, nil, nil, newTestEncryption(), nil, nil,
		oauth2session.Config{CredentialFiles: config.ThirdPartyOAuth2.CredentialFiles}, logger).
		WithCredentialFileReader(reader).WithClientIdentityRepository(repo)
	t.Cleanup(func() { require.NoError(t, sessions.Close(context.Background())) })

	initialRead, resume := make(chan struct{}), make(chan struct{})
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(resume) }) })
	var reads atomic.Int32
	repo.afterGet = func(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) {
		// ValidateUpdate reads first; the second read is Update's own snapshot.
		if reads.Add(1) == 2 {
			assert.Equal(t, before.ClientIdentityRevision, entity.ClientIdentityRevision)
			close(initialRead)
			select {
			case <-resume:
			case <-ctx.Done():
			}
		}
	}
	body := credentialSourceHandlerBody()
	body["credential_source"] = "filesystem"
	body["display_name"] = "Metadata committed after observation"
	body["protected_resources"] = []string{}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "/api/services/"+stored.ID.String(), bytes.NewReader(encoded)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", strongETag(before.Version))
	response := httptest.NewRecorder()
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		router.ServeHTTP(response, request)
	}()
	select {
	case <-initialRead:
	case <-ctx.Done():
		t.Fatal("metadata PUT never reached its pre-mutation read barrier")
	}
	assert.Zero(t, reader.idReads, "metadata validation must not read the client-ID file")
	assert.Zero(t, reader.pairReads, "metadata validation must not read credentials")

	expires := time.Now().Add(time.Hour)
	unusable, err := sessions.RequiresReauthentication(ctx, []*storage.UserSession{{
		ID: id.NewSessionID(), ServiceID: stored.ID, Principal: "owner@example.test", AccessTokenExpiresAt: &expires, UpstreamClientID: &oldID,
	}}, ports.UsabilityGate)
	require.NoError(t, err)
	assert.True(t, unusable[stored.ID])
	observed, err := repo.InMemoryThirdpartyOAuth2ProviderRepository.Get(ctx, stored.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Version, observed.Version, "observation must not consume the administrative ETag")
	assert.Equal(t, before.UpdatedAt, observed.UpdatedAt)
	assert.Equal(t, int64(1), observed.ClientIdentityRevision)
	require.NotNil(t, observed.ClientIdentity)
	assert.Equal(t, id.ClientID(reader.identity), *observed.ClientIdentity)

	release.Do(func() { close(resume) })
	select {
	case <-completed:
	case <-ctx.Done():
		t.Fatal("metadata PUT did not finish after releasing its read barrier")
	}
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.False(t, repo.identityChanged, "metadata PUT must return the lock-derived no-increment outcome")
	after, err := repo.Get(ctx, stored.ID)
	require.NoError(t, err)
	assert.Equal(t, observed.ClientIdentity, after.ClientIdentity)
	assert.Equal(t, observed.ClientIdentityRevision, after.ClientIdentityRevision)
	assert.Equal(t, before.Version+1, after.Version)
	assert.Equal(t, strongETag(after.Version), response.Header().Get("ETag"))
	assertIdentityResponseHidden(t, response)
	credentialSourceHandlerAssertReadViews(t, router, stored.ID.String(), model.CredentialSourceFilesystem, "")
	assert.Equal(t, 1, reader.idReads, "only the intervening usability observation reads a file")
	assert.Zero(t, reader.pairReads)
	events := identityAuditEvents(t, &logs)
	require.Len(t, events, 1, "the successful metadata PUT must not steal attribution from the observation")
	assert.Equal(t, "INFO", events[0]["level"])
	assert.Equal(t, stored.ID.String(), events[0]["service_id"])
	assert.Equal(t, float64(1), events[0]["client_identity_revision"])
	assert.Equal(t, "filesystem_observation", events[0]["channel"])
	assert.NotContains(t, events[0], "operator_principal")
	assert.NotContains(t, events[0], "principal")
	assert.Empty(t, *typedOperators)
	for _, sensitive := range []string{oldID.String(), reader.identity, "private-file-secret", binding.ClientIDFile, binding.ClientSecretFile, filepath.Base(binding.ClientIDFile), filepath.Base(binding.ClientSecretFile)} {
		assert.NotContains(t, logs.String(), sensitive)
		assert.NotContains(t, response.Body.String(), sensitive)
	}
}

func TestServicesHandler_ClientIdentitySameIDRotationAndSourceTransitionsEmitNoChange(t *testing.T) {
	for _, tc := range []struct {
		name       string
		initial    model.CredentialSource
		result     model.CredentialSource
		invalid    bool
		omitSecret bool
	}{
		{"secret rotation", model.CredentialSourceStored, model.CredentialSourceStored, false, false},
		{"metadata and scopes", model.CredentialSourceStored, model.CredentialSourceStored, false, false},
		{"stored to filesystem", model.CredentialSourceStored, model.CredentialSourceFilesystem, false, false},
		{"filesystem metadata", model.CredentialSourceFilesystem, model.CredentialSourceFilesystem, false, false},
		{"filesystem to stored", model.CredentialSourceFilesystem, model.CredentialSourceStored, false, false},
		{"reverse transition still requires explicit secret", model.CredentialSourceFilesystem, model.CredentialSourceStored, true, true},
		{"filesystem still rejects inline credentials", model.CredentialSourceStored, model.CredentialSourceFilesystem, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &identityBoundaryRepository{InMemoryThirdpartyOAuth2ProviderRepository: memory.NewInMemoryThirdpartyOAuth2ProviderRepository()}
			stored := credentialSourceHandlerStoredFixture(t, repo)
			stored.ClientID = "current-client-after-change"
			changed, err := repo.Update(t.Context(), stored, &stored.Version)
			require.NoError(t, err)
			require.True(t, changed)
			if tc.initial == model.CredentialSourceFilesystem {
				stored.CredentialSource = model.CredentialSourceFilesystem
				stored.ClientID = ""
				stored.Secret = model.NewAbsentSecret()
				changed, err = repo.Update(t.Context(), stored, &stored.Version)
				require.NoError(t, err)
				require.False(t, changed)
			}
			before, err := repo.Get(t.Context(), stored.ID)
			require.NoError(t, err)
			require.Equal(t, int64(1), before.ClientIdentityRevision)
			repo.identityChanged = false
			var logs bytes.Buffer
			logger, operators := identityAuditLogger(t, &logs)
			config := testConfig()
			config.ThirdPartyOAuth2.CredentialFiles = map[string]ports.CredentialFileBinding{
				"credential-source": {ClientIDFile: "/unavailable/private-id-basename", ClientSecretFile: "/unavailable/private-secret-basename"},
			}
			router, provider := identityAuditRouter(t, repo, config, logger)
			reader := &identityMetadataFileReader{identity: "current-client-after-change"}
			sessions := oauth2session.NewOAuth2SessionService(provider, nil, nil, nil, nil, newTestEncryption(), nil, nil,
				oauth2session.Config{CredentialFiles: config.ThirdPartyOAuth2.CredentialFiles}, logger).
				WithCredentialFileReader(reader).WithClientIdentityRepository(repo)
			t.Cleanup(func() { require.NoError(t, sessions.Close(context.Background())) })
			body := credentialSourceHandlerBody()
			body["display_name"] = "Updated metadata"
			body["credential_source"] = tc.result
			body["scopes"] = []map[string]string{{"scope_value": "read", "description": "Updated scope metadata"}}
			if tc.result == model.CredentialSourceStored || tc.invalid && !tc.omitSecret {
				body["client_id"] = reader.identity
				body["client_secret"] = "private-new-same-ID-secret"
			}
			if tc.omitSecret {
				delete(body, "client_secret")
			}

			response := credentialSourceHandlerRequest(t, router, http.MethodPut, "/api/services/"+stored.ID.String(), body, "")

			wantStatus := http.StatusOK
			if tc.invalid {
				wantStatus = http.StatusBadRequest
			}
			require.Equal(t, wantStatus, response.Code, response.Body.String())
			assertIdentityResponseHidden(t, response)
			assert.False(t, repo.identityChanged)
			assert.Zero(t, reader.idReads, "administrative metadata writes must not resolve files")
			assert.Zero(t, reader.pairReads)
			after, err := repo.Get(t.Context(), stored.ID)
			require.NoError(t, err)
			assert.Equal(t, before.ClientIdentity, after.ClientIdentity)
			assert.Equal(t, before.ClientIdentityRevision, after.ClientIdentityRevision)
			if tc.invalid {
				assert.Equal(t, before, after)
			} else {
				assert.Equal(t, tc.result, after.CredentialSource)
				assert.Equal(t, before.Version+1, after.Version)
				if tc.result == model.CredentialSourceStored {
					read, err := provider.Get(t.Context(), stored.ID)
					require.NoError(t, err)
					secret, err := read.Secret.GetPlaintext()
					require.NoError(t, err)
					assert.Equal(t, "private-new-same-ID-secret", secret)
				}
				identity := id.ClientID(reader.identity)
				unusable, err := sessions.RequiresReauthentication(t.Context(), []*storage.UserSession{{
					ServiceID: stored.ID, UpstreamClientID: &identity, UpstreamClientRevision: before.ClientIdentityRevision,
				}}, ports.UsabilityGate)
				require.NoError(t, err)
				assert.False(t, unusable[stored.ID], "same effective identity and revision remain usable")
			}
			assert.Empty(t, identityAuditEvents(t, &logs))
			assert.Empty(t, *operators)
			for _, sensitive := range []string{"private-new-same-ID-secret", "private-id-basename", "private-secret-basename"} {
				assert.NotContains(t, logs.String(), sensitive)
				assert.NotContains(t, response.Body.String(), sensitive)
			}
		})
	}
}
