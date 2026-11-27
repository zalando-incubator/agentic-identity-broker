package thirdparty

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ---- doubles ----

type fakeServiceAccountRepo struct {
	accounts   map[id.ServiceAccountID]*model.ServiceAccount
	writes     int
	createErr  error
	updateErr  error
	deleteErr  error
	lastUpdate *model.ServiceAccount
	// beforeWrite runs once at the start of the next Update or Delete, to interleave a
	// concurrent writer between the service's read and its write.
	beforeWrite func()
	createHook  func()
}

func (r *fakeServiceAccountRepo) interleave() {
	if hook := r.beforeWrite; hook != nil {
		r.beforeWrite = nil
		hook()
	}
}

func newFakeServiceAccountRepo() *fakeServiceAccountRepo {
	return &fakeServiceAccountRepo{accounts: map[id.ServiceAccountID]*model.ServiceAccount{}}
}

func cloneAccount(account *model.ServiceAccount) *model.ServiceAccount {
	copied := *account
	copied.Scopes = slices.Clone(account.Scopes)
	return &copied
}

func (r *fakeServiceAccountRepo) notFound() error {
	return storage.NewStorageError("fake", storage.ErrorKindNotFound, ports.ErrNotFound, "service account not found")
}

func (r *fakeServiceAccountRepo) nameTaken(account *model.ServiceAccount) bool {
	for _, other := range r.accounts {
		if other.ID != account.ID && other.ServiceID == account.ServiceID && other.CanonicalID == account.CanonicalID {
			return true
		}
	}
	return false
}

func (r *fakeServiceAccountRepo) Create(_ context.Context, account *model.ServiceAccount) error {
	if r.createErr != nil {
		return r.createErr
	}
	if r.createHook != nil {
		r.createHook()
	}
	if r.nameTaken(account) {
		return storage.NewStorageError("fake", storage.ErrorKindConflict, ports.ErrServiceAccountCanonicalIDConflict, "taken")
	}
	r.writes++
	account.Version = 1
	r.accounts[account.ID] = cloneAccount(account)
	return nil
}

func (r *fakeServiceAccountRepo) Get(_ context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID) (*model.ServiceAccount, error) {
	account, ok := r.accounts[accountID]
	if !ok || account.ServiceID != serviceID {
		return nil, r.notFound()
	}
	return cloneAccount(account), nil
}

func (r *fakeServiceAccountRepo) GetByCanonicalID(_ context.Context, serviceID id.ServiceID, canonicalID string) (*model.ServiceAccount, error) {
	for _, account := range r.accounts {
		if account.ServiceID == serviceID && account.CanonicalID == canonicalID {
			return cloneAccount(account), nil
		}
	}
	return nil, r.notFound()
}

func (r *fakeServiceAccountRepo) ListByService(_ context.Context, serviceID id.ServiceID) ([]*model.ServiceAccount, error) {
	accounts := []*model.ServiceAccount{}
	for _, account := range r.accounts {
		if account.ServiceID == serviceID {
			accounts = append(accounts, cloneAccount(account))
		}
	}
	slices.SortFunc(accounts, func(a, b *model.ServiceAccount) int { return strings.Compare(a.CanonicalID, b.CanonicalID) })
	return accounts, nil
}

func (r *fakeServiceAccountRepo) version(account *model.ServiceAccount, expected *int64) error {
	if expected != nil && *expected != account.Version {
		return storage.NewStorageError("fake", storage.ErrorKindConflict, ports.ErrServiceAccountVersionMismatch, "stale")
	}
	return nil
}

func (r *fakeServiceAccountRepo) Update(_ context.Context, account *model.ServiceAccount, expected *int64) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.interleave()
	stored, ok := r.accounts[account.ID]
	if !ok || stored.ServiceID != account.ServiceID {
		return r.notFound()
	}
	if err := r.version(stored, expected); err != nil {
		return err
	}
	if r.nameTaken(account) {
		return storage.NewStorageError("fake", storage.ErrorKindConflict, ports.ErrServiceAccountCanonicalIDConflict, "taken")
	}
	r.writes++
	account.Version = stored.Version + 1
	r.accounts[account.ID] = cloneAccount(account)
	r.lastUpdate = cloneAccount(account)
	return nil
}

func (r *fakeServiceAccountRepo) Delete(_ context.Context, serviceID id.ServiceID, accountID id.ServiceAccountID, expected *int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.interleave()
	stored, ok := r.accounts[accountID]
	if !ok || stored.ServiceID != serviceID {
		return r.notFound()
	}
	if err := r.version(stored, expected); err != nil {
		return err
	}
	r.writes++
	delete(r.accounts, accountID)
	return nil
}

func (r *fakeServiceAccountRepo) CountByService(_ context.Context, serviceID id.ServiceID) (int, error) {
	accounts, _ := r.ListByService(context.Background(), serviceID)
	return len(accounts), nil
}

type recordingEncryption struct {
	plaintexts [][]byte
	contexts   []map[string]string
	decrypts   int
	err        error
}

func (e *recordingEncryption) Encrypt(_ context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}
	e.plaintexts = append(e.plaintexts, slices.Clone(plaintext))
	e.contexts = append(e.contexts, encryptionContext)
	return []byte("ciphertext-" + strconv.Itoa(len(e.plaintexts))), nil
}

func (e *recordingEncryption) Decrypt(context.Context, []byte, map[string]string) ([]byte, error) {
	e.decrypts++
	return nil, errors.New("service accounts never decrypt in this feature")
}

type fakeServiceLookup map[id.ServiceID]bool

func (l fakeServiceLookup) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if !l[serviceID] {
		return nil, storage.NewStorageError("GetThirdpartyOAuth2Provider", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID}, nil
}

type stubBindings struct {
	bound bool
	err   error
}

func (b stubBindings) IsBound(context.Context, id.ServiceAccountID) (bool, error) {
	return b.bound, b.err
}

// principalPolicyLogHandler stands in for the principal policy of LOGGING.md section 5.
type principalPolicyLogHandler struct{ next slog.Handler }

func (h principalPolicyLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h principalPolicyLogHandler) Handle(ctx context.Context, record slog.Record) error {
	rendered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		rendered.AddAttrs(policyRender(attr))
		return true
	})
	return h.next.Handle(ctx, rendered)
}

func (h principalPolicyLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return principalPolicyLogHandler{h.next.WithAttrs(attrs)}
}

func (h principalPolicyLogHandler) WithGroup(name string) slog.Handler {
	return principalPolicyLogHandler{h.next.WithGroup(name)}
}

func policyRender(attr slog.Attr) slog.Attr {
	if principal, ok := attr.Value.Any().(id.Principal); ok && attr.Value.Kind() == slog.KindLogValuer {
		return slog.String(attr.Key, "policy("+principal.String()+")")
	}
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		rendered := []slog.Attr{}
		for _, member := range value.Group() {
			rendered = append(rendered, policyRender(member))
		}
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(rendered...)}
	}
	return slog.Attr{Key: attr.Key, Value: value}
}

// ---- harness ----

const saOperator = id.Principal("ops-admin@example.com")

type saHarness struct {
	service    *ServiceAccountService
	repo       *fakeServiceAccountRepo
	encryption *recordingEncryption
	serviceID  id.ServiceID
	other      id.ServiceID
	now        time.Time
	logs       *bytes.Buffer
	bindings   *stubBindings
}

func newSAHarness(t *testing.T) *saHarness {
	t.Helper()
	h := &saHarness{
		repo: newFakeServiceAccountRepo(), encryption: &recordingEncryption{},
		serviceID: id.NewServiceID(), other: id.NewServiceID(),
		now:  time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC),
		logs: &bytes.Buffer{}, bindings: &stubBindings{},
	}
	logger := slog.New(principalPolicyLogHandler{slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})})
	h.service = NewServiceAccountService(h.repo, h.encryption, fakeServiceLookup{h.serviceID: true, h.other: true},
		h.bindings, func() time.Time { return h.now }, logger)
	return h
}

func (h *saHarness) records(t *testing.T) []map[string]any {
	t.Helper()
	records := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(h.logs.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func (h *saHarness) eventRecords(t *testing.T, prefix string) []map[string]any {
	t.Helper()
	matched := []map[string]any{}
	for _, record := range h.records(t) {
		if event, _ := record["event"].(string); strings.HasPrefix(event, prefix) {
			matched = append(matched, record)
		}
	}
	return matched
}

func (h *saHarness) assertLogsFreeOf(t *testing.T, jwt string) {
	t.Helper()
	for _, part := range append([]string{jwt}, strings.Split(jwt, ".")...) {
		if part != "" {
			assert.NotContains(t, h.logs.String(), part)
		}
	}
}

func saJWT(t *testing.T, kid string, claims map[string]any) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	headers := jws.NewHeaders()
	require.NoError(t, headers.Set("kid", kid))
	signed, err := jws.Sign(payload, jws.WithKey(jwa.ES256(), key, jws.WithProtectedHeaders(headers)))
	require.NoError(t, err)
	return string(signed)
}

func (h *saHarness) token(t *testing.T, kid string, exp time.Time) string {
	return saJWT(t, kid, map[string]any{"iss": "https://idp.example.com", "sub": "triage-bot", "exp": exp.Unix()})
}

func credentialInput(jwt string) *model.ServiceAccountCredentialInput {
	return &model.ServiceAccountCredentialInput{Format: model.CredentialFormatSignedJWT, JWT: model.NewPlaintextSecret(jwt)}
}

func (h *saHarness) createRequest(canonicalID, jwt string) ServiceAccountCreate {
	return ServiceAccountCreate{CanonicalID: canonicalID, Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read"}, Credential: credentialInput(jwt)}
}

func (h *saHarness) create(t *testing.T, canonicalID, jwt string) *ServiceAccountView {
	t.Helper()
	view, err := h.service.Create(context.Background(), saOperator, h.serviceID, h.createRequest(canonicalID, jwt))
	require.NoError(t, err)
	return view
}

func (h *saHarness) resetLogs() {
	h.logs.Reset()
}

func requireValidation(t *testing.T, err error, message string) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, message, err.Error())
	var credentialErr *model.CredentialValidationError
	var entityErr *model.ServiceAccountValidationError
	assert.True(t, errors.As(err, &credentialErr) || errors.As(err, &entityErr), "validation error type, got %T", err)
}

// ---- constructor ----

func TestNewServiceAccountService_RequiresLogger(t *testing.T) {
	assert.Panics(t, func() {
		NewServiceAccountService(newFakeServiceAccountRepo(), &recordingEncryption{}, fakeServiceLookup{}, NoServiceAccountBindings{}, time.Now, nil)
	})
}

// ---- User Story 1: Create, List, Get, Resolve ----

func TestServiceAccountService_Create(t *testing.T) {
	ctx := context.Background()
	h := newSAHarness(t)
	jwt := h.token(t, "2026-11", h.now.Add(36*24*time.Hour))

	view, err := h.service.Create(ctx, saOperator, h.serviceID, ServiceAccountCreate{
		CanonicalID: "platform-triage-bot", Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"write", "read"}, Credential: credentialInput(jwt),
	})

	require.NoError(t, err)
	account := view.Account
	assert.False(t, account.ID.IsZero())
	assert.Equal(t, h.serviceID, account.ServiceID)
	assert.Equal(t, []string{"read", "write"}, account.Scopes)
	assert.Equal(t, int64(1), account.Version)
	assert.Equal(t, h.now, account.CreatedAt)
	assert.Equal(t, h.now, account.UpdatedAt)
	assert.Equal(t, model.ServiceAccountStatusReady, view.Status)
	assert.Nil(t, view.FailureReason)
	assert.Equal(t, model.NarrowingNone, view.Narrowing)
	assert.Equal(t, model.CredentialFingerprint(jwt), account.Summary.Fingerprint)
	assert.Equal(t, "2026-11", *account.Summary.Kid)
	assert.True(t, account.Credential.IsEncrypted())

	require.Len(t, h.encryption.contexts, 1)
	assert.Equal(t, domainencryption.NewServiceBranchKeySubject(h.serviceID).EncryptionContext(), h.encryption.contexts[0])
	assert.Equal(t, map[string]string{"service_id": h.serviceID.String()}, h.encryption.contexts[0])
	var document map[string]string
	require.NoError(t, json.Unmarshal(h.encryption.plaintexts[0], &document))
	assert.Equal(t, map[string]string{"format": "signed_jwt", "use": "access_token", "jwt": jwt}, document)
	stored := h.repo.accounts[account.ID]
	ciphertext, err := stored.Credential.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, []byte("ciphertext-1"), ciphertext, "the repository receives ciphertext only")

	records := h.eventRecords(t, "service.service_account.")
	require.Len(t, records, 1)
	record := records[0]
	assert.Equal(t, "service.service_account.created", record["event"])
	assert.Equal(t, "INFO", record["level"])
	assert.Equal(t, "policy(ops-admin@example.com)", record["operator_principal"])
	assert.Equal(t, h.serviceID.String(), record["service_id"])
	assert.Equal(t, account.ID.String(), record["service_account_id"])
	assert.Equal(t, "platform-triage-bot", record["canonical_id"])
	assert.Equal(t, "jwt_bearer", record["kind"])
	assert.Equal(t, []any{"read", "write"}, record["scopes"])
	credential := record["credential"].(map[string]any)
	assert.Equal(t, account.Summary.Fingerprint, credential["fingerprint"])
	assert.Equal(t, "policy(triage-bot)", credential["sub"])
	assert.NotContains(t, record, "changes")
	h.assertLogsFreeOf(t, jwt)
	assert.NotContains(t, h.logs.String(), "ciphertext-1")
}

func TestServiceAccountService_CreateFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown service is not found and stores nothing", func(t *testing.T) {
		h := newSAHarness(t)
		jwt := h.token(t, "k", h.now.Add(time.Hour))
		_, err := h.service.Create(ctx, saOperator, id.NewServiceID(), h.createRequest("platform-triage-bot", jwt))
		assert.True(t, ports.IsNotFoundErr(err))
		assert.Zero(t, h.repo.writes)
		assert.Empty(t, h.encryption.plaintexts)
	})

	t.Run("name conflict writes no audit record", func(t *testing.T) {
		h := newSAHarness(t)
		h.create(t, "platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()
		jwt := h.token(t, "k2", h.now.Add(time.Hour))
		_, err := h.service.Create(ctx, saOperator, h.serviceID, h.createRequest("platform-triage-bot", jwt))
		assert.ErrorIs(t, err, ports.ErrServiceAccountCanonicalIDConflict)
		assert.Empty(t, h.eventRecords(t, "service.service_account."))
		assert.Len(t, h.repo.accounts, 1)
		h.assertLogsFreeOf(t, jwt)
	})

	t.Run("encryption failure stores nothing and logs write_failed without the cause", func(t *testing.T) {
		h := newSAHarness(t)
		h.encryption.err = errors.New("kms unavailable: plaintext eyJleaked")
		jwt := h.token(t, "k", h.now.Add(time.Hour))
		_, err := h.service.Create(ctx, saOperator, h.serviceID, h.createRequest("platform-triage-bot", jwt))
		require.Error(t, err)
		assert.Zero(t, h.repo.writes)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.write_failed", records[0]["event"])
		assert.Equal(t, "ERROR", records[0]["level"])
		assert.Equal(t, "encryption", records[0]["error_kind"])
		assert.Equal(t, "create", records[0]["operation"])
		assert.Equal(t, "policy(ops-admin@example.com)", records[0]["operator_principal"])
		assert.NotContains(t, h.logs.String(), "kms unavailable")
		assert.NotContains(t, err.Error(), "eyJleaked")
		h.assertLogsFreeOf(t, jwt)
	})

	t.Run("storage failure logs write_failed with the error kind", func(t *testing.T) {
		h := newSAHarness(t)
		h.repo.createErr = storage.NewStorageError("fake", storage.ErrorKindConnection, errors.New("dial tcp"), "down")
		_, err := h.service.Create(ctx, saOperator, h.serviceID, h.createRequest("platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour))))
		require.Error(t, err)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.write_failed", records[0]["event"])
		assert.Equal(t, "connection", records[0]["error_kind"])
		assert.NotContains(t, h.logs.String(), "dial tcp")
	})
}

func TestServiceAccountService_ListGetResolve(t *testing.T) {
	ctx := context.Background()
	h := newSAHarness(t)
	triageJWT := h.token(t, "k", h.now.Add(time.Hour))
	triage := h.create(t, "platform-triage-bot", triageJWT)
	planner := h.create(t, "platform-planner-bot", h.token(t, "k", h.now.Add(time.Hour)))
	h.encryption.plaintexts = nil

	views, err := h.service.List(ctx, h.serviceID)
	require.NoError(t, err)
	require.Len(t, views, 2)
	assert.Equal(t, []string{"platform-planner-bot", "platform-triage-bot"}, []string{views[0].Account.CanonicalID, views[1].Account.CanonicalID})
	assert.Equal(t, model.ServiceAccountStatusReady, views[0].Status)
	assert.Equal(t, model.NarrowingNone, views[1].Narrowing)

	empty, err := h.service.List(ctx, h.other)
	require.NoError(t, err)
	assert.Empty(t, empty)
	_, err = h.service.List(ctx, id.NewServiceID())
	assert.True(t, ports.IsNotFoundErr(err), "list under an unknown service is not found")

	byID, err := h.service.Resolve(ctx, h.serviceID, triage.Account.ID.String())
	require.NoError(t, err)
	assert.Equal(t, triage.Account.ID, byID)
	byName, err := h.service.Resolve(ctx, h.serviceID, "platform-planner-bot")
	require.NoError(t, err)
	assert.Equal(t, planner.Account.ID, byName)
	_, err = h.service.Resolve(ctx, h.other, "platform-planner-bot")
	assert.True(t, ports.IsNotFoundErr(err))

	got, err := h.service.Get(ctx, h.serviceID, triage.Account.ID)
	require.NoError(t, err)
	assert.Equal(t, triage.Account.Summary, got.Account.Summary)
	_, err = h.service.Get(ctx, h.other, triage.Account.ID)
	assert.True(t, ports.IsNotFoundErr(err), "an account of another service is not found")

	assert.Empty(t, h.encryption.plaintexts, "reads never touch the encryption port")
	assert.Zero(t, h.encryption.decrypts, "reads never decrypt")
	h.assertLogsFreeOf(t, triageJWT)
}

// ---- User Story 2: refusals ----

func TestServiceAccountService_CreateRefusals(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	valid := func(t *testing.T) string {
		return saJWT(t, "k", map[string]any{"exp": now.Add(time.Hour).Unix()})
	}
	tests := []struct {
		name    string
		request func(t *testing.T) ServiceAccountCreate
		message string
		reason  string
	}{
		{"unknown kind", func(t *testing.T) ServiceAccountCreate {
			return ServiceAccountCreate{CanonicalID: "a", Kind: "client_credentials", Scopes: []string{"read"}, Credential: credentialInput(valid(t))}
		}, "kind must be jwt_bearer", ""},
		{"invalid canonical_id", func(t *testing.T) ServiceAccountCreate {
			return ServiceAccountCreate{CanonicalID: "a b", Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read"}, Credential: credentialInput(valid(t))}
		}, "canonical_id must be 1-128 characters using only letters, digits, '.', '_', or '-'", ""},
		{"empty scopes", func(t *testing.T) ServiceAccountCreate {
			return ServiceAccountCreate{CanonicalID: "a", Kind: model.ServiceAccountKindJWTBearer, Credential: credentialInput(valid(t))}
		}, "scopes must be a non-empty list of unique, non-empty values", ""},
		{"missing credential", func(t *testing.T) ServiceAccountCreate {
			return ServiceAccountCreate{CanonicalID: "a", Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read"}}
		}, "credential is required", ""},
		{"unsigned credential", func(t *testing.T) ServiceAccountCreate {
			return ServiceAccountCreate{CanonicalID: "a", Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read"},
				Credential: credentialInput("eyJhbGciOiJub25lIn0.eyJleHAiOjk5OTk5OTk5OTl9.")}
		}, "credential.jwt must be a signed compact JWS", "unsigned"},
		{"expired credential", func(t *testing.T) ServiceAccountCreate {
			return ServiceAccountCreate{CanonicalID: "a", Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read"},
				Credential: credentialInput(saJWT(t, "k", map[string]any{"exp": now.Unix()}))}
		}, "credential.jwt is expired", "expired"},
		{"assertion use", func(t *testing.T) ServiceAccountCreate {
			input := credentialInput(valid(t))
			input.Use = model.CredentialUseAssertion
			return ServiceAccountCreate{CanonicalID: "a", Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read"}, Credential: input}
		}, "credential.use must be access_token", "unsupported_use"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newSAHarness(t)
			request := tc.request(t)

			_, err := h.service.Create(ctx, saOperator, h.serviceID, request)

			requireValidation(t, err, tc.message)
			assert.Zero(t, h.repo.writes)
			assert.Empty(t, h.encryption.plaintexts)
			assert.Empty(t, h.eventRecords(t, "service.service_account.created"))
			rejected := h.eventRecords(t, "service.service_account.credential_rejected")
			if tc.reason == "" {
				assert.Empty(t, rejected)
				return
			}
			require.Len(t, rejected, 1)
			assert.Equal(t, "WARN", rejected[0]["level"])
			assert.Equal(t, tc.reason, rejected[0]["reason"])
			assert.Equal(t, "create", rejected[0]["operation"])
			assert.Equal(t, h.serviceID.String(), rejected[0]["service_id"])
			assert.Equal(t, "policy(ops-admin@example.com)", rejected[0]["operator_principal"])
			assert.NotContains(t, rejected[0], "service_account_id")
			jwt, _ := request.Credential.JWT.GetPlaintext()
			h.assertLogsFreeOf(t, jwt)
			assert.NotContains(t, err.Error(), strings.Split(jwt, ".")[0])
		})
	}
}

// ---- User Story 3: Update ----

func TestServiceAccountService_UpdateOutcomes(t *testing.T) {
	ctx := context.Background()

	t.Run("rotation replaces credential and summary and audits previous and new", func(t *testing.T) {
		h := newSAHarness(t)
		oldJWT := h.token(t, "2026-11", h.now.Add(time.Hour))
		created := h.create(t, "platform-triage-bot", oldJWT)
		h.resetLogs()
		h.now = h.now.Add(time.Minute)
		newJWT := h.token(t, "2026-12", h.now.Add(48*time.Hour))

		view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, Credential: credentialInput(newJWT)})

		require.NoError(t, err)
		assert.Equal(t, ServiceAccountUpdated, outcome)
		assert.Equal(t, int64(2), view.Account.Version)
		assert.Equal(t, h.now, view.Account.UpdatedAt)
		assert.Equal(t, created.Account.CreatedAt, view.Account.CreatedAt)
		assert.Equal(t, model.CredentialFingerprint(newJWT), view.Account.Summary.Fingerprint)
		ciphertext, _ := h.repo.accounts[created.Account.ID].Credential.GetCiphertext()
		assert.Equal(t, []byte("ciphertext-2"), ciphertext)
		assert.Equal(t, map[string]string{"service_id": h.serviceID.String()}, h.encryption.contexts[1])

		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.credential_rotated", records[0]["event"])
		assert.Equal(t, []any{"credential"}, records[0]["changes"])
		assert.Equal(t, created.Account.Summary.Fingerprint, records[0]["previous_observed_credential"].(map[string]any)["fingerprint"])
		assert.Equal(t, "2026-11", records[0]["previous_observed_credential"].(map[string]any)["kid"])
		assert.Equal(t, "2026-12", records[0]["credential"].(map[string]any)["kid"])
		assert.NotContains(t, records[0], "previous_observed_scopes")
		h.assertLogsFreeOf(t, oldJWT)
		h.assertLogsFreeOf(t, newJWT)
	})

	t.Run("rotation returns a failed account to ready", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "platform-triage-bot", h.token(t, "k", h.now.Add(time.Second)))
		h.now = h.now.Add(time.Hour)
		failed, err := h.service.Status(ctx, h.serviceID, created.Account.ID)
		require.NoError(t, err)
		assert.Equal(t, model.ServiceAccountStatusFailed, failed.Status)

		view, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, Credential: credentialInput(h.token(t, "k2", h.now.Add(time.Hour)))})
		require.NoError(t, err)
		assert.Equal(t, model.ServiceAccountStatusReady, view.Status)
		assert.Nil(t, view.FailureReason)
	})

	t.Run("ceiling change keeps credential and summary", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()

		view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"write", "read"}})

		require.NoError(t, err)
		assert.Equal(t, ServiceAccountUpdated, outcome)
		assert.Equal(t, []string{"read", "write"}, view.Account.Scopes)
		assert.Equal(t, created.Account.Summary, view.Account.Summary)
		assert.Len(t, h.encryption.plaintexts, 1, "no encryption without a new credential")
		ciphertext, _ := h.repo.accounts[created.Account.ID].Credential.GetCiphertext()
		assert.Equal(t, []byte("ciphertext-1"), ciphertext)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.ceiling_changed", records[0]["event"])
		assert.Equal(t, []any{"scopes"}, records[0]["changes"])
		assert.Equal(t, []any{"read"}, records[0]["previous_observed_scopes"])
		assert.Equal(t, []any{"read", "write"}, records[0]["scopes"])
		assert.NotContains(t, records[0], "previous_observed_credential")
	})

	t.Run("rename changes only canonical_id", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()

		view, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{CanonicalID: new("platform-triage"), Scopes: []string{"read"}})

		require.NoError(t, err)
		assert.Equal(t, "platform-triage", view.Account.CanonicalID)
		assert.Equal(t, created.Account.ID, view.Account.ID)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.renamed", records[0]["event"])
		assert.Equal(t, []any{"canonical_id"}, records[0]["changes"])
		assert.Equal(t, "platform-triage-bot", records[0]["previous_observed_canonical_id"])
		assert.Equal(t, "platform-triage", records[0]["canonical_id"])
	})

	t.Run("combined change writes one record by precedence listing all changes", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()

		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{
			CanonicalID: new("platform-triage"), Scopes: []string{"admin"}, Credential: credentialInput(h.token(t, "k2", h.now.Add(time.Hour))),
		})

		require.NoError(t, err)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.credential_rotated", records[0]["event"])
		assert.Equal(t, []any{"credential", "scopes", "canonical_id"}, records[0]["changes"])
		for _, key := range []string{"previous_observed_canonical_id", "previous_observed_scopes", "previous_observed_credential"} {
			assert.Contains(t, records[0], key)
		}
		assert.Equal(t, 2, h.repo.writes)
	})

	t.Run("ceiling and rename without a rotation is a ceiling change", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()
		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{CanonicalID: new("x"), Scopes: []string{"admin"}})
		require.NoError(t, err)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.ceiling_changed", records[0]["event"])
		assert.Equal(t, []any{"scopes", "canonical_id"}, records[0]["changes"])
	})

	t.Run("no-op writes nothing and keeps version and updated_at", func(t *testing.T) {
		h := newSAHarness(t)
		jwt := h.token(t, "k", h.now.Add(time.Hour))
		created := h.create(t, "platform-triage-bot", jwt)
		h.resetLogs()
		h.now = h.now.Add(time.Minute)
		for name, request := range map[string]ServiceAccountUpdate{
			"same ceiling":                      {Scopes: []string{"read"}},
			"same name stated":                  {CanonicalID: new("platform-triage-bot"), Scopes: []string{"read"}},
			"resubmitted JWT":                   {Scopes: []string{"read"}, Credential: credentialInput(jwt)},
			"stated kind":                       {Kind: new(model.ServiceAccountKindJWTBearer), Scopes: []string{"read"}},
			"current version":                   {Scopes: []string{"read"}, ExpectedVersion: new(int64(1))},
			"resubmitted JWT with explicit use": {Scopes: []string{"read"}, Credential: &model.ServiceAccountCredentialInput{Format: model.CredentialFormatSignedJWT, Use: model.CredentialUseAccessToken, JWT: model.NewPlaintextSecret(jwt)}},
		} {
			view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, request)
			require.NoError(t, err, name)
			assert.Equal(t, ServiceAccountUnchanged, outcome, name)
			assert.Equal(t, int64(1), view.Account.Version, name)
			assert.Equal(t, created.Account.UpdatedAt, view.Account.UpdatedAt, name)
		}
		reordered := newSAHarness(t)
		account, err := reordered.service.Create(ctx, saOperator, reordered.serviceID, ServiceAccountCreate{
			CanonicalID: "a", Kind: model.ServiceAccountKindJWTBearer, Scopes: []string{"read", "write"}, Credential: credentialInput(reordered.token(t, "k", reordered.now.Add(time.Hour))),
		})
		require.NoError(t, err)
		reordered.resetLogs()
		_, outcome, err := reordered.service.Update(ctx, saOperator, reordered.serviceID, account.Account.ID, ServiceAccountUpdate{Scopes: []string{"write", "read"}})
		require.NoError(t, err)
		assert.Equal(t, ServiceAccountUnchanged, outcome, "reordering the ceiling is not a change")
		assert.Equal(t, 1, reordered.repo.writes)
		assert.Empty(t, reordered.eventRecords(t, "service.service_account."))
	})
}

func TestServiceAccountService_UpdateRefusals(t *testing.T) {
	ctx := context.Background()

	t.Run("kind change is refused; unknown kind is refused first", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Kind: new(model.ServiceAccountKind("client_credentials")), Scopes: []string{"read"}})
		requireValidation(t, err, "kind must be jwt_bearer")
		assert.Equal(t, 1, h.repo.writes)
	})

	t.Run("invalid credential leaves everything unchanged and writes credential_rejected", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()
		expired := h.token(t, "k", h.now.Add(-time.Second))

		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"admin"}, Credential: credentialInput(expired)})

		requireValidation(t, err, "credential.jwt is expired")
		assert.Equal(t, 1, h.repo.writes)
		assert.Equal(t, created.Account.Scopes, h.repo.accounts[created.Account.ID].Scopes)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.credential_rejected", records[0]["event"])
		assert.Equal(t, "update", records[0]["operation"])
		assert.Equal(t, "expired", records[0]["reason"])
		assert.Equal(t, created.Account.ID.String(), records[0]["service_account_id"])
		h.assertLogsFreeOf(t, expired)
	})

	t.Run("encryption failure during rotation leaves the stored account unchanged", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()
		h.encryption.err = errors.New("kms: secret detail")

		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, Credential: credentialInput(h.token(t, "k2", h.now.Add(time.Hour)))})

		require.Error(t, err)
		stored := h.repo.accounts[created.Account.ID]
		assert.Equal(t, int64(1), stored.Version)
		assert.Equal(t, created.Account.Summary, stored.Summary)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.write_failed", records[0]["event"])
		assert.Equal(t, "update", records[0]["operation"])
		assert.Equal(t, "encryption", records[0]["error_kind"])
		assert.NotContains(t, h.logs.String(), "secret detail")
	})

	t.Run("name conflict, stale version and foreign account", func(t *testing.T) {
		h := newSAHarness(t)
		first := h.create(t, "platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		h.create(t, "platform-planner-bot", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()

		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, first.Account.ID, ServiceAccountUpdate{CanonicalID: new("platform-planner-bot"), Scopes: []string{"read"}})
		assert.ErrorIs(t, err, ports.ErrServiceAccountCanonicalIDConflict)

		_, _, err = h.service.Update(ctx, saOperator, h.serviceID, first.Account.ID, ServiceAccountUpdate{Scopes: []string{"admin"}, ExpectedVersion: new(int64(2))})
		assert.ErrorIs(t, err, ports.ErrServiceAccountVersionMismatch)

		_, _, err = h.service.Update(ctx, saOperator, h.serviceID, first.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, ExpectedVersion: new(int64(2))})
		assert.ErrorIs(t, err, ports.ErrServiceAccountVersionMismatch, "a stale version is refused even when nothing would change")

		_, _, err = h.service.Update(ctx, saOperator, h.other, first.Account.ID, ServiceAccountUpdate{Scopes: []string{"admin"}})
		assert.True(t, ports.IsNotFoundErr(err))

		assert.Equal(t, 2, h.repo.writes)
		assert.Empty(t, h.eventRecords(t, "service.service_account."))
	})

	t.Run("request errors win over a stale version", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{ExpectedVersion: new(int64(9))})
		requireValidation(t, err, "scopes must be a non-empty list of unique, non-empty values")
		_, _, err = h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{CanonicalID: new("a b"), Scopes: []string{"read"}, ExpectedVersion: new(int64(9))})
		requireValidation(t, err, "canonical_id must be 1-128 characters using only letters, digits, '.', '_', or '-'")
	})
}

// ---- User Story 4: Status ----

func TestServiceAccountService_Status(t *testing.T) {
	ctx := context.Background()
	h := newSAHarness(t)
	expiry := h.now.Add(time.Hour)
	expiring := h.create(t, "expiring", h.token(t, "k", expiry))
	unbounded := h.create(t, "unbounded", saJWT(t, "k", map[string]any{"iss": "https://idp.example.com"}))
	h.resetLogs()
	writes := h.repo.writes

	at := func(instant time.Time) (*ServiceAccountView, []ServiceAccountView) {
		h.now = instant
		view, err := h.service.Status(ctx, h.serviceID, expiring.Account.ID)
		require.NoError(t, err)
		list, err := h.service.List(ctx, h.serviceID)
		require.NoError(t, err)
		return view, list
	}

	view, list := at(expiry.Add(-time.Nanosecond))
	assert.Equal(t, model.ServiceAccountStatusReady, view.Status)
	assert.Nil(t, view.FailureReason)
	assert.Equal(t, model.ServiceAccountStatusReady, list[0].Status)

	for _, instant := range []time.Time{expiry, expiry.Add(5 * time.Second)} {
		view, list = at(instant)
		assert.Equal(t, model.ServiceAccountStatusFailed, view.Status)
		require.NotNil(t, view.FailureReason)
		assert.Equal(t, model.ServiceAccountFailureCredentialExpired, *view.FailureReason)
		assert.Equal(t, model.ServiceAccountStatusFailed, list[0].Status, "the list reports the same status")
		assert.Equal(t, expiry, *view.Account.CredentialExpiresAt)
	}

	h.now = expiry.AddDate(10, 0, 0)
	never, err := h.service.Status(ctx, h.serviceID, unbounded.Account.ID)
	require.NoError(t, err)
	assert.Equal(t, model.ServiceAccountStatusReady, never.Status)
	assert.Nil(t, never.Account.CredentialExpiresAt)

	_, err = h.service.Status(ctx, h.other, expiring.Account.ID)
	assert.True(t, ports.IsNotFoundErr(err))
	assert.Equal(t, writes, h.repo.writes, "reads write nothing")
	assert.Empty(t, h.records(t), "reads log nothing")
}

// ---- User Story 5: Delete ----

func TestServiceAccountService_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("delete audits the pre-delete state after commit", func(t *testing.T) {
		h := newSAHarness(t)
		jwt := h.token(t, "k", h.now.Add(time.Hour))
		created := h.create(t, "platform-triage-bot", jwt)
		h.resetLogs()

		require.NoError(t, h.service.Delete(ctx, saOperator, h.serviceID, created.Account.ID, new(int64(1))))

		assert.Empty(t, h.repo.accounts)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.deleted", records[0]["event"])
		assert.Equal(t, "platform-triage-bot", records[0]["canonical_id"])
		assert.Equal(t, []any{"read"}, records[0]["scopes"])
		assert.Equal(t, created.Account.Summary.Fingerprint, records[0]["credential"].(map[string]any)["fingerprint"])
		assert.Equal(t, "policy(ops-admin@example.com)", records[0]["operator_principal"])
		assert.NotContains(t, records[0], "changes")
		h.assertLogsFreeOf(t, jwt)
	})

	t.Run("not found, stale version and bound account change nothing", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		h.resetLogs()

		assert.True(t, ports.IsNotFoundErr(h.service.Delete(ctx, saOperator, h.serviceID, id.NewServiceAccountID(), nil)))
		assert.True(t, ports.IsNotFoundErr(h.service.Delete(ctx, saOperator, h.other, created.Account.ID, nil)))
		assert.ErrorIs(t, h.service.Delete(ctx, saOperator, h.serviceID, created.Account.ID, new(int64(2))), ports.ErrServiceAccountVersionMismatch)

		h.bindings.bound = true
		err := h.service.Delete(ctx, saOperator, h.serviceID, created.Account.ID, nil)
		assert.ErrorIs(t, err, ErrServiceAccountBound)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
		assert.Equal(t, "service account is bound by an agent grant", storageErr.Message)

		h.bindings.bound, h.bindings.err = false, errors.New("grant store down")
		require.Error(t, h.service.Delete(ctx, saOperator, h.serviceID, created.Account.ID, nil))

		assert.Len(t, h.repo.accounts, 1)
		assert.Empty(t, h.eventRecords(t, "service.service_account.deleted"))
	})
}

// ---- concurrent writers and response status ----

// rotateConcurrently stores a rotation of the account, as a concurrent writer would, the next
// time the service writes.
func (h *saHarness) rotateConcurrently(accountID id.ServiceAccountID) {
	h.repo.beforeWrite = func() {
		rotated := cloneAccount(h.repo.accounts[accountID])
		rotated.Credential = model.NewEncryptedSecret([]byte("rotated-ciphertext"))
		rotated.Summary.Fingerprint = "SHA256:rotated"
		rotated.Version++
		h.repo.accounts[accountID] = rotated
	}
}

func TestServiceAccountService_ConcurrentWriters(t *testing.T) {
	ctx := context.Background()

	t.Run("an unconditional ceiling change keeps a credential rotated after its read", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		h.rotateConcurrently(created.Account.ID)
		h.resetLogs()

		view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read", "write"}})

		require.NoError(t, err)
		assert.Equal(t, ServiceAccountUpdated, outcome)
		stored := h.repo.accounts[created.Account.ID]
		assert.Equal(t, "SHA256:rotated", stored.Summary.Fingerprint)
		ciphertext, err := stored.Credential.GetCiphertext()
		require.NoError(t, err)
		assert.Equal(t, []byte("rotated-ciphertext"), ciphertext)
		assert.Equal(t, []string{"read", "write"}, stored.Scopes)
		assert.Equal(t, int64(3), view.Account.Version)
		assert.Equal(t, "SHA256:rotated", view.Account.Summary.Fingerprint)
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "SHA256:rotated", records[0]["credential"].(map[string]any)["fingerprint"])
	})

	t.Run("an unconditional ceiling change keeps a rename committed after its read", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		h.repo.beforeWrite = func() {
			renamed := cloneAccount(h.repo.accounts[created.Account.ID])
			renamed.CanonicalID = "renamed"
			renamed.Version++
			h.repo.accounts[created.Account.ID] = renamed
		}

		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read", "write"}})

		require.NoError(t, err)
		assert.Equal(t, "renamed", h.repo.accounts[created.Account.ID].CanonicalID)
		assert.Equal(t, []string{"read", "write"}, h.repo.accounts[created.Account.ID].Scopes)
	})

	t.Run("a stale If-Match is still refused when another writer intervenes", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		h.rotateConcurrently(created.Account.ID)

		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read", "write"}, ExpectedVersion: new(int64(1))})

		assert.ErrorIs(t, err, ports.ErrServiceAccountVersionMismatch)
		assert.Equal(t, "SHA256:rotated", h.repo.accounts[created.Account.ID].Summary.Fingerprint)
	})

	t.Run("an unconditional delete audits the credential it erased", func(t *testing.T) {
		h := newSAHarness(t)
		created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
		h.rotateConcurrently(created.Account.ID)
		h.resetLogs()

		require.NoError(t, h.service.Delete(ctx, saOperator, h.serviceID, created.Account.ID, nil))

		assert.Empty(t, h.repo.accounts)
		records := h.eventRecords(t, "service.service_account.deleted")
		require.Len(t, records, 1)
		assert.Equal(t, "SHA256:rotated", records[0]["credential"].(map[string]any)["fingerprint"])
	})
}

func TestServiceAccountService_ResponseStatusFollowsTheWrite(t *testing.T) {
	ctx := context.Background()

	t.Run("create reports failed when the credential expires while it is stored", func(t *testing.T) {
		h := newSAHarness(t)
		exp := h.now.Add(time.Second)
		h.repo.createHook = func() { h.now = exp }

		view, err := h.service.Create(ctx, saOperator, h.serviceID, h.createRequest("a", h.token(t, "k", exp)))

		require.NoError(t, err)
		assert.Equal(t, model.ServiceAccountStatusFailed, view.Status)
		assert.Equal(t, h.now.Add(-time.Second), view.Account.CreatedAt, "write timestamps keep the validation time")
	})

	t.Run("update reports failed when the credential expires while it is stored", func(t *testing.T) {
		h := newSAHarness(t)
		exp := h.now.Add(time.Hour)
		created := h.create(t, "a", h.token(t, "k", exp))
		h.repo.beforeWrite = func() { h.now = exp }

		view, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read", "write"}})

		require.NoError(t, err)
		assert.Equal(t, model.ServiceAccountStatusFailed, view.Status)
	})
}
