package app

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
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
