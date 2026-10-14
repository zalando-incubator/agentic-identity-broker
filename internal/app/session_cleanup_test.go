package app

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	domstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_SessionCleanupLifecycle(t *testing.T) {
	for _, mode := range []string{"local", "hybrid", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cfg := &ports.Config{
					Server: ports.ServerConfig{
						EndUser: ports.ServerInstanceConfig{PublicURL: "http://localhost:8000"},
					},
					Storage: ports.StorageConfig{
						Backend:  "memory",
						Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
					},
					ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
						JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
					},
					Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
					OAuth2AuthServer: ports.OAuth2AuthServerConfig{Mode: servermode.Mode(mode)},
				}
				if mode != "proxy" {
					cfg.OAuth2AuthServer.Local = ports.LocalModeConfig{TokenTTL: time.Hour}
				}
				if mode != "local" {
					cfg.OAuth2AuthServer.Proxy = ports.ProxyModeConfig{
						UpstreamIssuerURI:         "https://issuer.example.com",
						UpstreamAuthorizeEndpoint: "https://issuer.example.com/authorize",
						UpstreamTokenEndpoint:     "https://issuer.example.com/token",
					}
				}
				adapter, err := storage.NewAdapter(&cfg.Storage)
				require.NoError(t, err)
				seed := func(signature string, expiresAt time.Time) {
					t.Helper()
					require.NoError(t, adapter.AuthorizationCodes().Create(t.Context(), &domstorage.AuthorizationCode{
						ID: id.NewAuthorizationCodeID(), CodeHash: signature, ExpiresAt: expiresAt,
					}))
					require.NoError(t, adapter.PKCESessions().Create(t.Context(), &domstorage.PKCESession{
						Signature: signature, ExpiresAt: expiresAt,
					}))
					require.NoError(t, adapter.RefreshTokenSessions().Create(t.Context(), &domstorage.RefreshTokenSession{
						Signature: signature, ExpiresAt: expiresAt,
					}))
				}
				assertRemainingExpired := func(want int) {
					t.Helper()
					for _, deletion := range []func(context.Context) (int, error){
						adapter.AuthorizationCodes().DeleteExpired,
						adapter.PKCESessions().DeleteExpired,
						adapter.RefreshTokenSessions().DeleteExpired,
					} {
						count, err := deletion(t.Context())
						require.NoError(t, err)
						assert.Equal(t, want, count, "expired records remaining after scheduled sweep")
					}
				}
				seed("startup", time.Now().Add(-time.Second))
				seed("live", time.Now().Add(time.Hour))
				application, err := NewBuilder().WithConfig(cfg).WithStorage(adapter).
					WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))).
					WithJWKSPublisher(&mockJWKSPublisher{}).Build()
				require.NoError(t, err)
				defer func() { assert.NoError(t, application.Shutdown(context.Background())) }()
				synctest.Wait()
				remaining := 0
				if mode == "proxy" {
					remaining = 1
				}
				assertRemainingExpired(remaining)

				seed("periodic", time.Now().Add(time.Second))
				time.Sleep(time.Minute)
				synctest.Wait()
				assertRemainingExpired(remaining)
				_, err = adapter.AuthorizationCodes().FindByCodeHash(t.Context(), "live")
				require.NoError(t, err)
				_, err = adapter.PKCESessions().FindBySignature(t.Context(), "live")
				require.NoError(t, err)
				_, err = adapter.RefreshTokenSessions().FindBySignature(t.Context(), "live")
				require.NoError(t, err)

				require.NoError(t, application.Shutdown(context.Background()))
				seed("after-shutdown", time.Now().Add(-time.Second))
				time.Sleep(time.Minute)
				synctest.Wait()
				assertRemainingExpired(1)
			})
		})
	}
}

type shutdownRefreshRepository func(context.Context, id.Principal, id.ServiceID, func(context.Context, *domstorage.UserSession) (bool, error)) (*domstorage.UserSession, error)

func (r shutdownRefreshRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *domstorage.UserSession) (bool, error)) (*domstorage.UserSession, error) {
	return r(ctx, principal, serviceID, refresh)
}

type shutdownTokenTransport func(*http.Request) (*http.Response, error)

func (f shutdownTokenTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBuilder_ShutdownDrainsTokenCommitAndRejectsBackgroundWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := &ports.Config{
			Server: ports.ServerConfig{EndUser: ports.ServerInstanceConfig{PublicURL: "http://localhost:8000"}},
			Storage: ports.StorageConfig{
				Backend: "memory", Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
			},
			ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
				JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
			},
			Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
			OAuth2AuthServer: ports.OAuth2AuthServerConfig{Mode: "local", Local: ports.LocalModeConfig{TokenTTL: time.Hour}},
		}
		adapter, err := storage.NewAdapter(&cfg.Storage)
		require.NoError(t, err)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		application, err := NewBuilder().WithConfig(cfg).WithStorage(adapter).WithLogger(logger).Build()
		require.NoError(t, err)
		releaseCommit := make(chan struct{})
		defer func() {
			select {
			case <-releaseCommit:
			default:
				close(releaseCommit)
			}
			assert.NoError(t, application.Shutdown(context.Background()))
		}()
		encryption := testutil.NewTestEncryptionAdapter(t)
		committing := make(chan context.Context, 1)
		var locks, requests atomic.Int32
		refreshRepo := shutdownRefreshRepository(func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *domstorage.UserSession) (bool, error)) (*domstorage.UserSession, error) {
			locks.Add(1)
			return adapter.SessionRefresh().WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, current *domstorage.UserSession) (bool, error) {
				changed, err := refresh(ctx, current)
				if changed && err == nil {
					committing <- ctx
					select {
					case <-releaseCommit:
					case <-ctx.Done():
						return false, ctx.Err()
					}
				}
				return changed, err
			})
		})
		// Replace only the session service to gate its commit; retain the builder's shutdown chain.
		require.NoError(t, application.OAuth2SessionService.Close(context.Background()))
		application.OAuth2SessionService = oauth2session.NewOAuth2SessionService(
			application.ProviderService, adapter.UserSessions(), refreshRepo, nil, nil, encryption,
			&http.Client{Transport: shutdownTokenTransport(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"rotated-refresh","token_type":"Bearer","expires_in":3600}`)),
				}, nil
			})}, nil, oauth2session.DefaultConfig(), logger,
		)
		serviceID := id.NewServiceID()
		require.NoError(t, adapter.Services().Create(context.Background(), &model.ThirdpartyOAuth2ProviderEntity{
			ID: serviceID, ClientID: "client", DisplayName: "Provider", Secret: model.NewAbsentSecret(),
			TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
			Endpoints:               model.OAuth2Endpoints{TokenEndpoint: "https://provider.example/token"},
		}))
		seed := func(principal id.Principal) *domstorage.UserSession {
			t.Helper()
			binding := map[string]string{"service_id": serviceID.String()}
			access, err := encryption.Encrypt(context.Background(), []byte("old-access"), binding)
			require.NoError(t, err)
			refresh, err := encryption.Encrypt(context.Background(), []byte("old-refresh"), binding)
			require.NoError(t, err)
			expiry := time.Now().Add(2 * time.Minute)
			session := &domstorage.UserSession{
				ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID, TokenType: "Bearer",
				EncryptedAccessToken: access, EncryptedRefreshToken: refresh, AccessTokenExpiresAt: &expiry,
				EncryptionContext: domstorage.EncryptionContext{ServiceID: serviceID},
			}
			require.NoError(t, adapter.UserSessions().Create(context.Background(), session))
			return session
		}
		first, later := seed("drain-user"), seed("later-user")
		_, token, err := application.OAuth2SessionService.GetValidAccessToken(context.Background(), first.Principal, serviceID)
		require.NoError(t, err)
		assert.Equal(t, "old-access", token)
		refreshCtx := <-committing
		persisted, err := adapter.UserSessions().Get(context.Background(), first.ID)
		require.NoError(t, err)
		assert.Equal(t, first.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
		shutdown := make(chan error, 1)
		go func() { shutdown <- application.Shutdown(context.Background()) }()
		synctest.Wait()
		select {
		case err := <-shutdown:
			t.Fatalf("shutdown returned before the rotated token committed: %v", err)
		default:
		}
		assert.NoError(t, refreshCtx.Err(), "shutdown must not cancel an admitted token update")
		close(releaseCommit)
		require.NoError(t, <-shutdown)
		persisted, err = adapter.UserSessions().Get(context.Background(), first.ID)
		require.NoError(t, err)
		access, err := application.OAuth2SessionService.DecryptAccessToken(context.Background(), persisted)
		require.NoError(t, err)
		assert.Equal(t, "new-access", access)
		refresh, err := application.OAuth2SessionService.DecryptRefreshToken(context.Background(), persisted)
		require.NoError(t, err)
		assert.Equal(t, "rotated-refresh", refresh)

		_, token, err = application.OAuth2SessionService.GetValidAccessToken(context.Background(), later.Principal, serviceID)
		require.NoError(t, err)
		assert.Equal(t, "old-access", token)
		synctest.Wait()
		assert.Equal(t, int32(1), locks.Load(), "shutdown must reject background work for another due session")
		assert.Equal(t, int32(1), requests.Load())
		persisted, err = adapter.UserSessions().Get(context.Background(), later.ID)
		require.NoError(t, err)
		assert.Equal(t, later, persisted)
	})
}
