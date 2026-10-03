package oauth2session

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/require"
)

type ledgerSessionEncryptor struct{}

func (ledgerSessionEncryptor) Encrypt(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
	return append([]byte("encrypted:"), plaintext...), nil
}
func (ledgerSessionEncryptor) Decrypt(_ context.Context, ciphertext []byte, _ map[string]string) ([]byte, error) {
	if !bytes.HasPrefix(ciphertext, []byte("encrypted:")) {
		return nil, errors.New("invalid test ciphertext")
	}
	return bytes.Clone(ciphertext[len("encrypted:"):]), nil
}

type ledgerSessionProviders struct {
	ports.ThirdpartyOAuth2ProviderRepository
	entity *model.ThirdpartyOAuth2ProviderEntity
}

func (r ledgerSessionProviders) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if serviceID != r.entity.ID {
		return nil, ports.ErrNotFound
	}
	return r.entity.Copy(), nil
}

type ledgerSessionRecords struct {
	ports.UserSessionRepository
	session *storage.UserSession
}

func copyLedgerSession(session *storage.UserSession) *storage.UserSession {
	if session == nil {
		return nil
	}
	copy := *session
	copy.EncryptedAccessToken, copy.EncryptedRefreshToken = bytes.Clone(session.EncryptedAccessToken), bytes.Clone(session.EncryptedRefreshToken)
	copy.Scope = append([]string(nil), session.Scope...)
	return &copy
}
func (r *ledgerSessionRecords) FindByPrincipalAndService(_ context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	if r.session == nil || r.session.Principal != principal || r.session.ServiceID != serviceID {
		return nil, nil
	}
	return copyLedgerSession(r.session), nil
}
func (r *ledgerSessionRecords) Create(_ context.Context, session *storage.UserSession) error {
	r.session = copyLedgerSession(session)
	return nil
}
func (r *ledgerSessionRecords) DeleteByPrincipalAndService(context.Context, id.Principal, id.ServiceID) error {
	r.session = nil
	return nil
}

func (r *ledgerSessionRecords) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	session, err := r.FindByPrincipalAndService(ctx, principal, serviceID)
	if err != nil || session == nil {
		return session, err
	}
	updated, err := refresh(ctx, session)
	if err != nil {
		return nil, err
	}
	if updated {
		r.session = copyLedgerSession(session)
	}
	return session, nil
}

func (r *ledgerSessionRecords) UpdateRefreshedSession(_ context.Context, previous, current *storage.UserSession) error {
	if r.session == nil || r.session.ID != previous.ID || !r.session.UpdatedAt.Equal(previous.UpdatedAt) ||
		!bytes.Equal(r.session.EncryptedAccessToken, previous.EncryptedAccessToken) ||
		!bytes.Equal(r.session.EncryptedRefreshToken, previous.EncryptedRefreshToken) {
		return storage.NewStorageError("UpdateRefreshedSession", storage.ErrorKindConflict, nil, "session changed during refresh")
	}
	r.session = copyLedgerSession(current)
	return nil
}

type ledgerSessionBranchKeys struct{ ports.BranchKeyManager }

func newLedgerSession(t *testing.T) (*OAuth2SessionService, *ledgerSessionRecords, *ledgerfixture.Store, *HandleCallbackRequest, *atomic.Bool) {
	t.Helper()
	failed := &atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if failed.Load() {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"upstream-error-canary"}`)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"access-token-canary","refresh_token":"refresh-token-canary","token_type":"Bearer","expires_in":3600,"scope":"read"}`)
	}))
	t.Cleanup(server.Close)
	serviceID := id.NewServiceID()
	entity := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, ClientID: "public-client", DisplayName: "Provider", Secret: model.NewAbsentSecret(), Endpoints: model.OAuth2Endpoints{AuthorizeEndpoint: server.URL + "/authorize", TokenEndpoint: server.URL + "/token"}, Scopes: []model.OAuthScope{{ScopeValue: "read"}}}
	entity.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	provider := thirdparty.NewThirdpartyOAuth2ProviderService(ledgerSessionProviders{entity: entity}, ledgerSessionEncryptor{}, ledgerSessionBranchKeys{}, nil, true, logger)
	repo := &ledgerSessionRecords{}
	store := &ledgerfixture.Store{Snapshot: func() func() { before := copyLedgerSession(repo.session); return func() { repo.session = before } }}
	key, err := jwk.Import[jwk.Key]([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	svc := NewOAuth2SessionService(provider, repo, repo, nil, nil, ledgerSessionEncryptor{}, server.Client(), domjwe.New(key), Config{CallbackBaseURL: "https://broker.example", StateTokenTTL: time.Minute, MaxRetries: 1}, logger, store.Recorder(t), store)
	state, err := svc.CreateStateToken(&OAuth2StateTokenClaims{Principal: "ledger-user", ServiceID: serviceID, PKCEVerifier: "pkce-verifier-canary", RedirectURI: "https://broker.example/sessions", IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute)})
	require.NoError(t, err)
	return svc, repo, store, &HandleCallbackRequest{ServiceID: serviceID, Code: "authorization-code-canary", State: state}, failed
}

func TestSessionLedgerCallbackReauthorizationAndTermination(t *testing.T) {
	svc, _, store, request, _ := newLedgerSession(t)
	ctx := context.Background()
	first, err := svc.HandleCallback(ctx, "ledger-user", request)
	require.NoError(t, err)
	second, err := svc.HandleCallback(ctx, "ledger-user", request)
	require.NoError(t, err)
	require.Equal(t, first.Session.ID, second.Session.ID)
	require.NoError(t, svc.TerminateSession(ctx, "ledger-user", request.ServiceID))
	require.ErrorIs(t, svc.TerminateSession(ctx, "ledger-user", request.ServiceID), ErrSessionNotFound)
	require.Len(t, store.Events, 3)
	for i, name := range []string{"session-established", "session-established", "session-terminated"} {
		event := store.Events[i]
		require.Equal(t, model.BusinessEventTypePrefix+name, event.Type)
		require.Equal(t, first.Session.ID, event.SessionID)
		require.Equal(t, request.ServiceID, event.ServiceID)
		require.Equal(t, id.Principal("ledger-user"), *event.Subject)
		require.Empty(t, event.Data)
	}
}

func TestSessionLedgerRecordingFailureRestoresSession(t *testing.T) {
	for _, action := range []string{"establish", "refresh", "terminate"} {
		for _, failure := range []string{"append", "commit"} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				svc, repo, store, request, _ := newLedgerSession(t)
				ctx := context.Background()
				if action != "establish" {
					_, err := svc.HandleCallback(ctx, "ledger-user", request)
					require.NoError(t, err)
					store.Events = nil
				}
				if action == "refresh" {
					expired := time.Now().Add(-time.Minute)
					repo.session.AccessTokenExpiresAt = &expired
				}
				before := copyLedgerSession(repo.session)
				failed := errors.New("ledger unavailable")
				if failure == "append" {
					store.AppendError = failed
				} else {
					store.CommitError = failed
				}
				var err error
				switch action {
				case "establish":
					result, failure := svc.HandleCallback(ctx, "ledger-user", request)
					err = failure
					require.Nil(t, result)
				case "refresh":
					_, _, err = svc.GetValidAccessToken(ctx, "ledger-user", request.ServiceID)
				case "terminate":
					err = svc.TerminateSession(ctx, "ledger-user", request.ServiceID)
				}
				require.Error(t, err)
				require.Equal(t, before, repo.session)
				require.Empty(t, store.Events)
			})
		}
	}
}

func TestSessionLedgerTerminalRefreshFailureKeepsOriginalSession(t *testing.T) {
	svc, repo, store, request, failed := newLedgerSession(t)
	_, err := svc.HandleCallback(context.Background(), "ledger-user", request)
	require.NoError(t, err)
	store.Events = nil
	expired := time.Now().Add(-time.Minute)
	repo.session.AccessTokenExpiresAt = &expired
	before := copyLedgerSession(repo.session)
	failed.Store(true)
	_, _, err = svc.GetValidAccessToken(context.Background(), "ledger-user", request.ServiceID)
	require.ErrorIs(t, err, ErrRefreshFailed)
	require.Equal(t, before, repo.session)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"session-refresh-failed", store.Events[0].Type)
	require.Equal(t, before.ID, store.Events[0].SessionID)
	require.Equal(t, before.ServiceID, store.Events[0].ServiceID)
	require.Equal(t, map[string]any{"reason_code": "upstream_rejected"}, store.Events[0].Data)
}

func TestSessionLedgerAcceptedRefreshSurvivesLaterExchangeRollback(t *testing.T) {
	svc, repo, store, request, _ := newLedgerSession(t)
	ctx := context.Background()
	_, err := svc.HandleCallback(ctx, "ledger-user", request)
	require.NoError(t, err)
	store.Events = nil
	expired := time.Now().Add(-time.Minute)
	repo.session.AccessTokenExpiresAt = &expired
	_, _, err = svc.GetValidAccessToken(ctx, "ledger-user", request.ServiceID)
	require.NoError(t, err)
	accepted := copyLedgerSession(repo.session)
	owner, err := store.BeginTX(ctx)
	require.NoError(t, err)
	repo.session.TokenType = "uncommitted-exchange-change"
	require.NoError(t, store.Rollback(owner))
	require.Equal(t, accepted, repo.session)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"session-refreshed", store.Events[0].Type)
	require.Equal(t, accepted.ID, store.Events[0].SessionID)
	require.Equal(t, accepted.ServiceID, store.Events[0].ServiceID)
}

func TestSessionLedgerAutomaticRefreshActor(t *testing.T) {
	for _, failure := range []bool{false, true} {
		outcome := "success"
		if failure {
			outcome = "failure"
		}
		t.Run(outcome, func(t *testing.T) {
			for _, caller := range []string{"delegated", "direct-user", "calling-peer-only", "no-context", "anonymous-capture"} {
				t.Run(caller, func(t *testing.T) {
					svc, repo, store, request, upstreamRejects := newLedgerSession(t)
					_, err := svc.HandleCallback(context.Background(), "ledger-user", request)
					require.NoError(t, err)
					store.Events = nil
					expired := time.Now().Add(-time.Minute)
					repo.session.AccessTokenExpiresAt = &expired
					before := copyLedgerSession(repo.session)
					require.False(t, before.HasValidAccessToken())
					require.True(t, before.CanRefresh())
					upstreamRejects.Store(failure)

					ctx := context.Background()
					peer := "verified-gateway-client"
					if caller != "no-context" {
						holder := security.NewCaptureHolder(security.TransportCapture{})
						ctx = security.WithCaptureHolder(ctx, holder)
						callingPeer, principalActor := "", before.Principal.String()
						if caller == "delegated" || caller == "calling-peer-only" {
							callingPeer = peer
						}
						if caller == "anonymous-capture" {
							principalActor = ""
						}
						resolved, finalized := security.FinalizeCaptureHolder(ctx, principalActor, callingPeer)
						require.True(t, finalized)
						require.Equal(t, security.NormalizeActor(principalActor), resolved.Actor)
						require.Equal(t, callingPeer, resolved.CallingPeer)
					}
					if caller == "delegated" {
						// The exchange workflow may supply these facts only after the grant
						// and requested service have both been authorized.
						ctx = ledger.WithWorkflowActor(ctx, model.BusinessEventActor{
							Kind: "gateway", ID: &peer, OnBehalfOf: &before.Principal,
						})
					}

					refreshed, _, err := svc.GetValidAccessToken(ctx, before.Principal, before.ServiceID)
					if failure {
						require.ErrorIs(t, err, ErrRefreshFailed)
						require.Nil(t, refreshed)
						require.Equal(t, before, repo.session)
					} else {
						require.NoError(t, err)
						require.Equal(t, before.ID, refreshed.ID)
						require.True(t, refreshed.HasValidAccessToken())
					}

					require.Len(t, store.Events, 1)
					event := store.Events[0]
					require.Equal(t, before.Principal, *event.Subject)
					require.Equal(t, before.ID, event.SessionID)
					require.Equal(t, before.ServiceID, event.ServiceID)
					if failure {
						require.Equal(t, model.BusinessEventTypePrefix+"session-refresh-failed", event.Type)
						require.Equal(t, map[string]any{"reason_code": "upstream_rejected"}, event.Data)
					} else {
						require.Equal(t, model.BusinessEventTypePrefix+"session-refreshed", event.Type)
						require.Empty(t, event.Data)
					}
					if caller == "delegated" {
						require.Equal(t, "gateway", event.Actor.Kind)
						require.Equal(t, peer, *event.Actor.ID)
						require.Equal(t, before.Principal, *event.Actor.OnBehalfOf)
						require.Equal(t, id.ClientID(peer), event.GatewayClientID)
					} else {
						require.Equal(t, "user", event.Actor.Kind)
						if caller == "no-context" || caller == "anonymous-capture" {
							require.Nil(t, event.Actor.ID, "a known session subject does not authenticate its caller")
						} else {
							require.Equal(t, before.Principal.String(), *event.Actor.ID)
						}
						require.Nil(t, event.Actor.OnBehalfOf, "a calling peer alone does not establish delegation")
						require.True(t, event.GatewayClientID.IsZero())
					}
				})
			}
		})
	}
}
