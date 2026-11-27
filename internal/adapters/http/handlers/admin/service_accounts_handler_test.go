package admin

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
)

// countingServiceAccountRepo counts every repository call, to prove nothing is read before
// authentication.
type countingServiceAccountRepo struct {
	ports.ServiceAccountRepository
	calls   int
	readErr error
}

func (r *countingServiceAccountRepo) Create(ctx context.Context, a *model.ServiceAccount) error {
	r.calls++
	return r.ServiceAccountRepository.Create(ctx, a)
}

func (r *countingServiceAccountRepo) Get(ctx context.Context, s id.ServiceID, a id.ServiceAccountID) (*model.ServiceAccount, error) {
	r.calls++
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.ServiceAccountRepository.Get(ctx, s, a)
}

func (r *countingServiceAccountRepo) GetByCanonicalID(ctx context.Context, s id.ServiceID, c string) (*model.ServiceAccount, error) {
	r.calls++
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.ServiceAccountRepository.GetByCanonicalID(ctx, s, c)
}

func (r *countingServiceAccountRepo) ListByService(ctx context.Context, s id.ServiceID) ([]*model.ServiceAccount, error) {
	r.calls++
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.ServiceAccountRepository.ListByService(ctx, s)
}

func (r *countingServiceAccountRepo) Update(ctx context.Context, a *model.ServiceAccount, v *int64) error {
	r.calls++
	return r.ServiceAccountRepository.Update(ctx, a, v)
}

func (r *countingServiceAccountRepo) Delete(ctx context.Context, s id.ServiceID, a id.ServiceAccountID, v *int64) error {
	r.calls++
	return r.ServiceAccountRepository.Delete(ctx, s, a, v)
}

type boundBindings struct{ bound bool }

func (b *boundBindings) IsBound(context.Context, id.ServiceAccountID) (bool, error) {
	return b.bound, nil
}

type saHandlerHarness struct {
	router    chi.Router
	repo      *countingServiceAccountRepo
	serviceID id.ServiceID
	other     id.ServiceID
	logs      *bytes.Buffer
	spans     *tracetest.SpanRecorder
	bindings  *boundBindings
	tokens    []string
}

const saTestOperator = "ops-admin@example.com"

func newSAHandlerHarness(t *testing.T) *saHandlerHarness {
	t.Helper()
	ctx := context.Background()
	providers := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	accounts := memory.NewServiceAccountRepository().WithProviderRepository(providers)
	providers.WithServiceAccountRepository(accounts)
	h := &saHandlerHarness{repo: &countingServiceAccountRepo{ServiceAccountRepository: accounts}, logs: &bytes.Buffer{}, bindings: &boundBindings{}}
	for i, canonicalID := range []string{"company-platform", "other-platform"} {
		name := canonicalID
		service := &model.ThirdpartyOAuth2ProviderEntity{
			ID: id.NewServiceID(), CanonicalID: &name, DisplayName: name, ClientID: id.ClientID(name),
			Secret: model.NewEncryptedSecret([]byte("opaque")), IssuerURI: "https://idp.example.test",
			Endpoints: model.OAuth2Endpoints{TokenEndpoint: "https://idp.example.test/token", AuthorizeEndpoint: "https://idp.example.test/authorize"},
		}
		require.NoError(t, providers.Create(ctx, service))
		if i == 0 {
			h.serviceID = service.ID
		} else {
			h.other = service.ID
		}
	}
	h.spans = tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(h.spans))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	logger := slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	encryption := testutil.NewTestEncryptionAdapter(t)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(providers, encryption, &encryptionnoop.BranchKeyManager{}, nil, false, logger)
	accountService := thirdparty.NewServiceAccountService(h.repo, encryption, providers, h.bindings, time.Now, logger)
	handler := NewServiceAccountsHandler(accountService, providerService, logger)
	router := chi.NewRouter()
	router.Route("/api/services/{service-id}/service-accounts", func(r chi.Router) {
		r.Post("/", handler.Create)
		r.Get("/", handler.List)
		r.Get("/{service-account-id}", handler.Get)
		r.Put("/{service-account-id}", handler.Update)
		r.Delete("/{service-account-id}", handler.Delete)
		r.Get("/{service-account-id}/status", handler.Status)
	})
	h.router = router
	return h
}

func (h *saHandlerHarness) jwt(t *testing.T, kid string, exp time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	headers := jws.NewHeaders()
	require.NoError(t, headers.Set("kid", kid))
	payload := `{"iss":"https://idp.example.com","sub":"triage-bot","exp":` + strconv.FormatInt(exp.Unix(), 10) + `}`
	signed, err := jws.Sign([]byte(payload), jws.WithKey(jwa.ES256(), key, jws.WithProtectedHeaders(headers)))
	require.NoError(t, err)
	h.tokens = append(h.tokens, string(signed))
	return string(signed)
}

func (h *saHandlerHarness) do(t *testing.T, method, path, operator string, headers map[string]string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	if operator != "" {
		req = req.WithContext(principal.WithPrincipal(req.Context(), id.NewPrincipal(operator)))
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

func (h *saHandlerHarness) path(serviceRef string, rest ...string) string {
	return "/api/services/" + serviceRef + "/service-accounts" + strings.Join(rest, "")
}

func createBody(canonicalID, jwt string) string {
	return `{"canonical_id":"` + canonicalID + `","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt","use":"access_token","jwt":"` + jwt + `"}}`
}

func (h *saHandlerHarness) create(t *testing.T, canonicalID string) (map[string]any, string) {
	t.Helper()
	jwt := h.jwt(t, "2026-11", time.Now().Add(time.Hour))
	w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, createBody(canonicalID, jwt))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return decodeObject(t, w), jwt
}

func decodeObject(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var object map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &object), w.Body.String())
	return object
}

func assertErrorBody(t *testing.T, w *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, ErrorResponse{Error: code, Message: message}, body)
}

// assertNoCredential checks responses, logs and spans for every JWT the harness created.
func (h *saHandlerHarness) assertNoCredential(t *testing.T, responses ...*httptest.ResponseRecorder) {
	t.Helper()
	var spanText strings.Builder
	for _, span := range h.spans.Ended() {
		spanText.WriteString(span.Name() + span.Status().Description)
		for _, attribute := range span.Attributes() {
			spanText.WriteString(attribute.Value.String())
		}
		for _, event := range span.Events() {
			spanText.WriteString(event.Name)
			for _, attribute := range event.Attributes {
				spanText.WriteString(attribute.Value.String())
			}
		}
	}
	for _, jwt := range h.tokens {
		for _, part := range append([]string{jwt}, strings.Split(jwt, ".")...) {
			if part == "" {
				continue
			}
			for _, w := range responses {
				assert.NotContains(t, w.Body.String(), part)
				for _, values := range w.Header() {
					assert.NotContains(t, strings.Join(values, ","), part)
				}
			}
			assert.NotContains(t, h.logs.String(), part)
			assert.NotContains(t, spanText.String(), part)
		}
	}
}

// ---- User Story 1 ----

func TestServiceAccountsHandler_RequiresOperatorBeforeAnyRead(t *testing.T) {
	h := newSAHandlerHarness(t)
	jwt := h.jwt(t, "k", time.Now().Add(time.Hour))
	account := h.path("company-platform", "/platform-triage-bot")
	for _, request := range []struct{ method, path, body string }{
		{http.MethodPost, h.path("company-platform"), createBody("a", jwt)},
		{http.MethodGet, h.path("company-platform"), ""},
		{http.MethodGet, account, ""},
		{http.MethodPut, account, `{"scopes":["read"]}`},
		{http.MethodDelete, account, ""},
		{http.MethodGet, account + "/status", ""},
	} {
		w := h.do(t, request.method, request.path, "", map[string]string{"If-Match": "W/bad"}, request.body)
		assertErrorBody(t, w, http.StatusUnauthorized, "unauthorized", "authentication required")
		h.assertNoCredential(t, w)
	}
	assert.Zero(t, h.repo.calls)
}

func TestServiceAccountsHandler_CreateListGet(t *testing.T) {
	h := newSAHandlerHarness(t)
	jwt := h.jwt(t, "2026-11", time.Now().Add(time.Hour))

	created := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, createBody("platform-triage-bot", jwt))

	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	assert.Equal(t, `"1"`, created.Header().Get("ETag"))
	assert.Equal(t, "application/json", created.Header().Get("Content-Type"))
	body := decodeObject(t, created)
	accountID := body["id"].(string)
	assert.Equal(t, "/api/services/"+h.serviceID.String()+"/service-accounts/"+accountID, created.Header().Get("Location"))
	assert.Equal(t, h.serviceID.String(), body["service_id"])
	assert.Equal(t, "none", body["narrowing"])
	assert.NotContains(t, body, "status", "status is read from the status resource only")
	assert.NotContains(t, body, "failure_reason")
	assert.NotContains(t, body, "credential")
	summary := body["credential_summary"].(map[string]any)
	assert.Equal(t, model.CredentialFingerprint(jwt), summary["fingerprint"])
	assert.Equal(t, "2026-11", summary["kid"])
	createdAt, err := time.Parse(time.RFC3339, body["created_at"].(string))
	require.NoError(t, err)
	assert.Equal(t, time.UTC, createdAt.Location())

	h.create(t, "platform-planner-bot")
	listed := h.do(t, http.MethodGet, h.path(h.serviceID.String()), saTestOperator, nil, "")
	require.Equal(t, http.StatusOK, listed.Code)
	var list []map[string]any
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &list))
	require.Len(t, list, 2)
	assert.Equal(t, "platform-planner-bot", list[0]["canonical_id"])
	assert.Equal(t, body, list[1])

	for _, ref := range []string{accountID, "platform-triage-bot"} {
		got := h.do(t, http.MethodGet, h.path("company-platform", "/"+ref), saTestOperator, nil, "")
		require.Equal(t, http.StatusOK, got.Code)
		assert.Equal(t, `"1"`, got.Header().Get("ETag"))
		assert.Equal(t, body, decodeObject(t, got))
	}
	empty := h.do(t, http.MethodGet, h.path("other-platform"), saTestOperator, nil, "")
	require.Equal(t, http.StatusOK, empty.Code)
	assert.JSONEq(t, `[]`, empty.Body.String())
	h.assertNoCredential(t, created, listed, empty)
}

func TestServiceAccountsHandler_NotFound(t *testing.T) {
	h := newSAHandlerHarness(t)
	created, _ := h.create(t, "platform-triage-bot")
	jwt := h.jwt(t, "k", time.Now().Add(time.Hour))
	cases := []struct{ method, path, body string }{
		{http.MethodPost, h.path(id.NewServiceID().String()), createBody("a", jwt)},
		{http.MethodPost, h.path("unknown-platform"), createBody("a", jwt)},
		{http.MethodGet, h.path(id.NewServiceID().String()), ""},
		{http.MethodGet, h.path("unknown-platform"), ""},
		{http.MethodGet, h.path("other-platform", "/"+created["id"].(string)), ""},
		{http.MethodGet, h.path("company-platform", "/unknown-bot"), ""},
		{http.MethodGet, h.path("company-platform", "/"+id.NewServiceID().String()+"/status"), ""},
		{http.MethodPut, h.path("other-platform", "/"+created["id"].(string)), `{"scopes":["read"]}`},
		{http.MethodDelete, h.path("company-platform", "/unknown-bot"), ""},
	}
	for _, tc := range cases {
		w := h.do(t, tc.method, tc.path, saTestOperator, nil, tc.body)
		assert.Equal(t, http.StatusNotFound, w.Code, tc.method+" "+tc.path)
		h.assertNoCredential(t, w)
	}
}

func TestServiceAccountsHandler_RequestBodyLimits(t *testing.T) {
	h := newSAHandlerHarness(t)
	jwt := h.jwt(t, "k", time.Now().Add(time.Hour))
	oversized := `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt","jwt":"` + strings.Repeat("a", serviceAccountMaxBodyBytes) + `"}}`
	for name, body := range map[string]string{
		"over 512 KiB":                   oversized,
		"unknown member":                 `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credentials":{"format":"signed_jwt","jwt":"` + jwt + `"}}`,
		"unknown credential member":      `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt","token":"` + jwt + `"}}`,
		"not JSON":                       `{"canonical_id":`,
		"trailing data":                  createBody("a", jwt) + `{}`,
		"scopes of the wrong type":       `{"canonical_id":"a","kind":"jwt_bearer","scopes":"read","credential":{"format":"signed_jwt","jwt":"` + jwt + `"}}`,
		"credential of the wrong type":   `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":"` + jwt + `"}`,
		"canonical_id of the wrong type": `{"canonical_id":5,"kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt","jwt":"` + jwt + `"}}`,
	} {
		w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, body)
		require.Equal(t, http.StatusBadRequest, w.Code, name)
		envelope := decodeObject(t, w)
		assert.Equal(t, "invalid request body", envelope["error"], name)
		h.assertNoCredential(t, w)
	}
	assert.Zero(t, h.repo.calls, "nothing is stored or read for a refused body")
}

// ---- User Story 2 ----

func TestServiceAccountsHandler_RefusalsMapToValidationFailed(t *testing.T) {
	h := newSAHandlerHarness(t)
	jwt := h.jwt(t, "k", time.Now().Add(time.Hour))
	expired := h.jwt(t, "k", time.Now().Add(-time.Hour))
	credential := `{"format":"signed_jwt","jwt":"` + jwt + `"}`
	cases := []struct{ name, body, message string }{
		{"unknown kind", `{"canonical_id":"a","kind":"client_credentials","scopes":["read"],"credential":` + credential + `}`, "kind must be jwt_bearer"},
		{"missing kind", `{"canonical_id":"a","scopes":["read"],"credential":` + credential + `}`, "kind must be jwt_bearer"},
		{"unknown format", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"private_key","jwt":"` + jwt + `"}}`, "credential.format must be signed_jwt"},
		{"use assertion", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt","use":"assertion","jwt":"` + jwt + `"}}`, "credential.use must be access_token"},
		{"empty scopes", `{"canonical_id":"a","kind":"jwt_bearer","scopes":[],"credential":` + credential + `}`, "scopes must be a non-empty list of unique, non-empty values"},
		{"missing scopes", `{"canonical_id":"a","kind":"jwt_bearer","credential":` + credential + `}`, "scopes must be a non-empty list of unique, non-empty values"},
		{"duplicate scopes", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read","read"],"credential":` + credential + `}`, "scopes must be a non-empty list of unique, non-empty values"},
		{"missing credential", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"]}`, "credential is required"},
		{"missing jwt", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt"}}`, "credential.jwt is required"},
		{"invalid canonical_id", `{"canonical_id":"a b","kind":"jwt_bearer","scopes":["read"],"credential":` + credential + `}`, "canonical_id must be 1-128 characters using only letters, digits, '.', '_', or '-'"},
		{"UUID-shaped canonical_id", `{"canonical_id":"` + id.NewServiceID().String() + `","kind":"jwt_bearer","scopes":["read"],"credential":` + credential + `}`, "canonical_id must not be UUID-shaped"},
		{"expired", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt","jwt":"` + expired + `"}}`, "credential.jwt is expired"},
		{"null scopes", `{"canonical_id":"a","kind":"jwt_bearer","scopes":null,"credential":` + credential + `}`, "scopes must not be null"},
		{"null credential", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":null}`, "credential must not be null"},
		{"null canonical_id", `{"canonical_id":null,"kind":"jwt_bearer","scopes":["read"],"credential":` + credential + `}`, "canonical_id must not be null"},
		{"null kind", `{"canonical_id":"a","kind":null,"scopes":["read"],"credential":` + credential + `}`, "kind must not be null"},
		{"null use", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt","use":null,"jwt":"` + jwt + `"}}`, "credential.use must not be null"},
		{"null jwt", `{"canonical_id":"a","kind":"jwt_bearer","scopes":["read"],"credential":{"format":"signed_jwt","jwt":null}}`, "credential.jwt must not be null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, tc.body)
			assertErrorBody(t, w, http.StatusBadRequest, "validation failed", tc.message)
			h.assertNoCredential(t, w)
		})
	}
	listed := h.do(t, http.MethodGet, h.path("company-platform"), saTestOperator, nil, "")
	assert.JSONEq(t, `[]`, listed.Body.String())
}

func TestServiceAccountsHandler_CreateNameConflict(t *testing.T) {
	h := newSAHandlerHarness(t)
	h.create(t, "platform-triage-bot")
	jwt := h.jwt(t, "k", time.Now().Add(time.Hour))
	w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, createBody("platform-triage-bot", jwt))
	assertErrorBody(t, w, http.StatusConflict, "conflict", "service account canonical_id already exists on this service")
	h.assertNoCredential(t, w)
}

// ---- User Story 3 ----

func TestServiceAccountsHandler_Update(t *testing.T) {
	h := newSAHandlerHarness(t)
	created, _ := h.create(t, "platform-triage-bot")
	h.create(t, "platform-planner-bot")
	path := h.path("company-platform", "/platform-triage-bot")

	unchanged := h.do(t, http.MethodPut, path, saTestOperator, nil, `{"scopes":["read"]}`)
	require.Equal(t, http.StatusOK, unchanged.Code, unchanged.Body.String())
	assert.Equal(t, `"1"`, unchanged.Header().Get("ETag"))
	assert.Equal(t, created, decodeObject(t, unchanged))

	stale := h.do(t, http.MethodPut, path, saTestOperator, map[string]string{"If-Match": `"2"`}, `{"scopes":["admin"]}`)
	assertErrorBody(t, stale, http.StatusPreconditionFailed, "precondition failed", "service account was changed; read it again and retry")
	for _, malformed := range []string{`W/"1"`, `1`, `"0"`, `"x"`, `*`, `"1", "2"`} {
		w := h.do(t, http.MethodPut, path, saTestOperator, map[string]string{"If-Match": malformed}, `{"scopes":["admin"]}`)
		assertErrorBody(t, w, http.StatusBadRequest, "invalid If-Match", "If-Match must be one strong ETag")
	}
	invalidAndStale := h.do(t, http.MethodPut, path, saTestOperator, map[string]string{"If-Match": `"9"`}, `{"scopes":[]}`)
	assertErrorBody(t, invalidAndStale, http.StatusBadRequest, "validation failed", "scopes must be a non-empty list of unique, non-empty values")

	jwt := h.jwt(t, "2026-12", time.Now().Add(2*time.Hour))
	rotated := h.do(t, http.MethodPut, path, saTestOperator, map[string]string{"If-Match": `"1"`},
		`{"scopes":["write","read"],"credential":{"format":"signed_jwt","jwt":"`+jwt+`"}}`)
	require.Equal(t, http.StatusOK, rotated.Code, rotated.Body.String())
	assert.Equal(t, `"2"`, rotated.Header().Get("ETag"))
	body := decodeObject(t, rotated)
	assert.Equal(t, created["id"], body["id"])
	assert.Equal(t, []any{"read", "write"}, body["scopes"])
	assert.Equal(t, model.CredentialFingerprint(jwt), body["credential_summary"].(map[string]any)["fingerprint"])

	for name, tc := range map[string]struct {
		body, message string
	}{
		"missing scopes":  {`{"canonical_id":"platform-triage-bot"}`, "scopes must be a non-empty list of unique, non-empty values"},
		"null credential": {`{"scopes":["read"],"credential":null}`, "credential must not be null"},
		"null canonical":  {`{"scopes":["read"],"canonical_id":null}`, "canonical_id must not be null"},
		"null kind":       {`{"scopes":["read"],"kind":null}`, "kind must not be null"},
		"kind changed":    {`{"scopes":["read"],"kind":"client_credentials"}`, "kind must be jwt_bearer"},
	} {
		w := h.do(t, http.MethodPut, path, saTestOperator, nil, tc.body)
		require.Equal(t, http.StatusBadRequest, w.Code, name)
		assert.Equal(t, tc.message, decodeObject(t, w)["message"], name)
	}

	conflict := h.do(t, http.MethodPut, path, saTestOperator, nil, `{"canonical_id":"platform-planner-bot","scopes":["read"]}`)
	assertErrorBody(t, conflict, http.StatusConflict, "conflict", "service account canonical_id already exists on this service")

	renamed := h.do(t, http.MethodPut, path, saTestOperator, nil, `{"canonical_id":"platform-triage","scopes":["read","write"]}`)
	require.Equal(t, http.StatusOK, renamed.Code)
	assert.Equal(t, `"3"`, renamed.Header().Get("ETag"))
	assert.Equal(t, http.StatusNotFound, h.do(t, http.MethodGet, path, saTestOperator, nil, "").Code)
	h.assertNoCredential(t, unchanged, stale, rotated, conflict, renamed)
}

// ---- User Story 4 ----

func TestServiceAccountsHandler_Status(t *testing.T) {
	h := newSAHandlerHarness(t)
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	jwt := h.jwt(t, "k", exp)
	created := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, createBody("platform-triage-bot", jwt))
	require.Equal(t, http.StatusCreated, created.Code)

	w := h.do(t, http.MethodGet, h.path("company-platform", "/platform-triage-bot/status"), saTestOperator, nil, "")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Values("ETag"), "status is derived at read time and carries no ETag, like discovery status")
	assert.JSONEq(t, `{"status":"ready","failure_reason":null,"credential_expires_at":"`+exp.UTC().Format(time.RFC3339)+`"}`, w.Body.String())

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	unbounded, err := jws.Sign([]byte(`{"iss":"x"}`), jws.WithKey(jwa.ES256(), key))
	require.NoError(t, err)
	h.tokens = append(h.tokens, string(unbounded))
	require.Equal(t, http.StatusCreated, h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, createBody("unbounded", string(unbounded))).Code)
	none := h.do(t, http.MethodGet, h.path("company-platform", "/unbounded/status"), saTestOperator, nil, "")
	assert.JSONEq(t, `{"status":"ready","failure_reason":null,"credential_expires_at":null}`, none.Body.String())
	h.assertNoCredential(t, created, w, none)
}

// ---- User Story 5 ----

func TestServiceAccountsHandler_Delete(t *testing.T) {
	h := newSAHandlerHarness(t)
	h.create(t, "platform-triage-bot")
	path := h.path("company-platform", "/platform-triage-bot")

	malformed := h.do(t, http.MethodDelete, path, saTestOperator, map[string]string{"If-Match": `W/"1"`}, "")
	assertErrorBody(t, malformed, http.StatusBadRequest, "invalid If-Match", "If-Match must be one strong ETag")
	stale := h.do(t, http.MethodDelete, path, saTestOperator, map[string]string{"If-Match": `"2"`}, "")
	assertErrorBody(t, stale, http.StatusPreconditionFailed, "precondition failed", "service account was changed; read it again and retry")

	h.bindings.bound = true
	bound := h.do(t, http.MethodDelete, path, saTestOperator, nil, "")
	assertErrorBody(t, bound, http.StatusConflict, "conflict", "service account is bound by an agent grant")
	h.bindings.bound = false

	deleted := h.do(t, http.MethodDelete, path, saTestOperator, map[string]string{"If-Match": `"1"`}, "")
	require.Equal(t, http.StatusNoContent, deleted.Code)
	assert.Empty(t, deleted.Body.String())
	assert.Equal(t, http.StatusNotFound, h.do(t, http.MethodDelete, path, saTestOperator, nil, "").Code)
	assert.Equal(t, http.StatusNotFound, h.do(t, http.MethodGet, path, saTestOperator, nil, "").Code)
	h.assertNoCredential(t, malformed, stale, bound, deleted)
}

func TestServiceAccountsHandler_ReadFailureIsLoggedOnceByKind(t *testing.T) {
	h := newSAHandlerHarness(t)
	created, _ := h.create(t, "platform-triage-bot")
	h.repo.readErr = storage.NewStorageError("GetServiceAccount", storage.ErrorKindConnection, errors.New("dial tcp 10.0.0.5:5432: stored-value"), "database unavailable")
	for _, path := range []string{
		h.path("company-platform"),
		h.path("company-platform", "/"+created["id"].(string)),
		h.path("company-platform", "/platform-triage-bot"),
		h.path("company-platform", "/"+created["id"].(string)+"/status"),
	} {
		h.logs.Reset()
		w := h.do(t, http.MethodGet, path, saTestOperator, nil, "")
		assertErrorBody(t, w, http.StatusInternalServerError, "internal server error", "")
		var failures []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(h.logs.String()), "\n") {
			var record map[string]any
			if json.Unmarshal([]byte(line), &record) == nil && record["msg"] == "service account read failed" {
				failures = append(failures, record)
			}
		}
		require.Len(t, failures, 1, path)
		assert.Equal(t, "ERROR", failures[0]["level"])
		assert.Equal(t, string(storage.ErrorKindConnection), failures[0]["error_kind"])
		assert.NotContains(t, h.logs.String(), "stored-value")
	}
}
