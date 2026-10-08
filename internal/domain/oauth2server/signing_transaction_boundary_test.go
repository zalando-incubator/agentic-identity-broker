package oauth2server

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/ory/fosite"
	"github.com/stretchr/testify/require"
)

type blockedSigningEncryptor struct {
	*testEncryptor
	started chan context.Context
	release chan struct{}
}

func (e *blockedSigningEncryptor) Encrypt(ctx context.Context, plaintext []byte, aad map[string]string) ([]byte, error) {
	e.started <- ctx
	select {
	case <-e.release:
		return e.testEncryptor.Encrypt(ctx, plaintext, aad)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func newSigningBoundaryFixture(t *testing.T) (*SigningKeyService, *memory.SigningKeyStore, *ledger.Service, *memory.TransactionManager) {
	t.Helper()
	manager := memory.NewTransactionManager()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	events := memory.NewBusinessEventRepository(manager, registry)
	recorder := ledger.NewService(registry, events, nil, manager, false)
	keys := memory.NewSigningKeyStore(manager)
	service := NewSigningKeyService(keys, keys, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger(), recorder)
	return service, keys, recorder, manager
}

func requireUnrelatedSigningRead(t *testing.T, recorder *ledger.Service) {
	t.Helper()
	read := make(chan error, 1)
	go func() {
		_, err := recorder.Query(context.Background(), model.BusinessEventQuery{
			Subject: model.BusinessEventSubject{Principal: id.Principal("unrelated-reader")},
			Start:   time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour),
		})
		read <- err
	}()
	synctest.Wait()
	select {
	case err := <-read:
		require.NoError(t, err)
	default:
		t.Fatal("unrelated read blocked behind signing I/O")
	}
}

func TestSigningPreparationDoesNotBlockUnrelatedReads(t *testing.T) {
	for _, operation := range []string{"generation", "bootstrap"} {
		for _, stage := range []string{"branch-provisioning", "encryption"} {
			t.Run(operation+"/"+stage, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					service, keys, recorder, _ := newSigningBoundaryFixture(t)
					started, release := make(chan context.Context, 1), make(chan struct{})
					releaseIO := sync.OnceFunc(func() { close(release) })
					defer releaseIO()
					var encryptor ports.EncryptionPort = &testEncryptor{}
					branchKeys := newNoopBranchKeyManager()
					if stage == "encryption" {
						encryptor = &blockedSigningEncryptor{testEncryptor: &testEncryptor{}, started: started, release: release}
					} else {
						branchKeys.createFn = func(ctx context.Context, _ domainencryption.BranchKeySubject) (string, error) {
							started <- ctx
							select {
							case <-release:
								return "", nil
							case <-ctx.Done():
								return "", ctx.Err()
							}
						}
					}
					service = NewSigningKeyService(keys, keys, encryptor, branchKeys, testSlogger(), recorder)
					done := make(chan error, 1)
					go func() {
						var err error
						if operation == "bootstrap" {
							_, _, err = service.EnsureInitialKey(context.Background(), "ES256")
						} else {
							_, err = service.GenerateAndStoreKey(context.Background(), "ES256", true)
						}
						done <- err
					}()
					ioCtx := <-started
					_, underTransaction := ports.StorageTransactionEffectsFromContext(ioCtx)
					require.False(t, underTransaction, "preparation must not inherit a ledger transaction")
					requireUnrelatedSigningRead(t, recorder)
					releaseIO()
					require.NoError(t, <-done)
					count, err := keys.CountActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
					require.NoError(t, err)
					require.Equal(t, 1, count)
				})
			})
		}
	}
}

func TestProviderColdSignerDoesNotBlockUnrelatedReads(t *testing.T) {
	for _, grant := range []string{"client_credentials", "authorization_code", "refresh_token"} {
		t.Run(grant, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				service, _, recorder, manager := newSigningBoundaryFixture(t)
				_, _, err := service.EnsureInitialKey(ctx, "ES256")
				require.NoError(t, err)
				agents := memory.NewAgentRepository(manager)
				provider, err := NewProvider(memory.NewAuthorizationCodeStore(manager), memory.NewRefreshTokenSessionStore(manager), memory.NewPKCESessionStore(manager), memory.NewClientCredentialStore(manager), &testClientResolver{agentRepo: agents}, service, "https://broker.example.com", time.Hour, time.Hour, "", testSlogger(), recorder, manager)
				require.NoError(t, err)
				agent, _, secret := setupTestCredentials(t, provider, agents)
				redirect, verifier := "http://localhost:8080/callback", "cold-signer-verifier-123456789012345678901234567890"
				var code, refreshToken string
				if grant != "client_credentials" {
					agent.RedirectURIs = []string{redirect}
					require.NoError(t, agents.Update(ctx, agent))
					code, err = provider.HandleAuthorize(ctx, agent.ID.String(), redirect, "code", "read offline_access", "state", generateS256Challenge(verifier), "S256", id.Principal("cold-signer-user"))
					require.NoError(t, err)
					if grant == "refresh_token" {
						response, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
						require.NoError(t, err)
						refreshToken = response.RefreshToken
					}
				}
				encryptor := &blockingSignerDecryptor{testEncryptor: &testEncryptor{}, started: make(chan context.Context, 1), release: make(chan struct{})}
				releaseIO := sync.OnceFunc(func() { close(encryptor.release) })
				defer releaseIO()
				service.encryption = encryptor
				service.invalidateSigner()
				type outcome struct {
					response *ports.TokenResponse
					err      error
				}
				done := make(chan outcome, 1)
				go func() {
					var response *ports.TokenResponse
					var err error
					switch grant {
					case "client_credentials":
						response, err = provider.HandleClientCredentials(ctx, agent.ID.String(), secret, "read")
					case "authorization_code":
						response, err = provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, verifier)
					case "refresh_token":
						response, err = provider.HandleRefreshToken(ctx, agent.ID.String(), secret, refreshToken, "")
					}
					done <- outcome{response, err}
				}()
				ioCtx := <-encryptor.started
				_, underTransaction := ports.StorageTransactionEffectsFromContext(ioCtx)
				require.False(t, underTransaction)
				requireUnrelatedSigningRead(t, recorder)
				select {
				case <-done:
					t.Fatal("token response escaped before signer preparation completed")
				default:
				}
				releaseIO()
				result := <-done
				require.NoError(t, result.err)
				require.NotEmpty(t, result.response.AccessToken)
				jwks, err := service.BuildJWKS(ctx)
				require.NoError(t, err)
				token, err := jwt.ParseString(result.response.AccessToken, jwt.WithKeySet(jwks), jwt.WithIssuer("https://broker.example.com"))
				require.NoError(t, err)
				scope, ok := token.Field("scope")
				require.True(t, ok)
				wantScope := "read"
				if grant != "client_credentials" {
					wantScope = "read offline_access"
				}
				require.Equal(t, wantScope, scope, "signing must use Fosite's final granted scopes")
				require.EqualValues(t, 1, encryptor.calls.Load(), "population must reuse the prepared signer")
			})
		})
	}
}

func TestSigningBootstrapPreparedLoserEmitsNoPromotion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, keys, recorder, _ := newSigningBoundaryFixture(t)
		started, release := make(chan context.Context, 2), make(chan struct{})
		releaseIO := sync.OnceFunc(func() { close(release) })
		defer releaseIO()
		encryptor := &blockedSigningEncryptor{testEncryptor: &testEncryptor{}, started: started, release: release}
		services := []*SigningKeyService{
			NewSigningKeyService(keys, keys, encryptor, newNoopBranchKeyManager(), testSlogger(), recorder),
			NewSigningKeyService(keys, keys, encryptor, newNoopBranchKeyManager(), testSlogger(), recorder),
		}
		type outcome struct {
			created bool
			err     error
		}
		done := make(chan outcome, 2)
		for _, service := range services {
			go func() {
				_, created, err := service.EnsureInitialKey(context.Background(), "ES256")
				done <- outcome{created, err}
			}()
		}
		synctest.Wait()
		require.Len(t, started, 2, "both candidates must prepare before bootstrap persistence coordination")
		releaseIO()
		winners := 0
		for range 2 {
			result := <-done
			require.NoError(t, result.err)
			if result.created {
				winners++
			}
		}
		require.Equal(t, 1, winners)
		_, created, err := service.EnsureInitialKey(context.Background(), "ES256")
		require.NoError(t, err)
		require.False(t, created)
		events, err := recorder.Query(context.Background(), model.BusinessEventQuery{Subject: model.BusinessEventSubject{NoSubject: true}, Type: model.BusinessEventTypePrefix + "signing-key-promoted", Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour)})
		require.NoError(t, err)
		require.Len(t, events, 1)
	})
}

func TestProviderPreparedSignerRevalidatesWithoutDecryptingAgain(t *testing.T) {
	for _, change := range []string{"cache-invalidated", "selection-changed"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			service, _, recorder, manager := newSigningBoundaryFixture(t)
			_, _, err := service.EnsureInitialKey(ctx, "ES256")
			require.NoError(t, err)
			standby, err := service.GenerateAndStoreKey(ctx, "ES256", false)
			require.NoError(t, err)
			encryptor := newCountingDecryptor()
			service.encryption = encryptor
			agents := memory.NewAgentRepository(manager)
			provider, err := NewProvider(memory.NewAuthorizationCodeStore(manager), memory.NewRefreshTokenSessionStore(manager), memory.NewPKCESessionStore(manager), memory.NewClientCredentialStore(manager), &testClientResolver{agentRepo: agents}, service, "https://broker.example.com", time.Hour, time.Hour, "", testSlogger(), recorder, manager)
			require.NoError(t, err)
			agent, _, _ := setupTestCredentials(t, provider, agents)
			client, err := provider.fositeStorage.GetClient(ctx, agent.ID.String())
			require.NoError(t, err)
			request := fosite.NewAccessRequest(&fosite.DefaultSession{Subject: agent.ID.String()})
			request.Client = client
			request.GrantTypes = fosite.Arguments{"client_credentials"}
			request.GrantScope("read")
			prepared, err := provider.accessStrategy.prepareSigningMaterial(ctx)
			require.NoError(t, err)
			service.invalidateSigner()
			if change == "selection-changed" {
				_, err = service.PromoteKey(ctx, standby.KID)
				require.NoError(t, err)
			}
			facts := model.BusinessEvent{Actor: model.BusinessEventActor{Kind: "agent"}, AgentID: agent.ID}
			response, err := provider.persistTokenResponse(ctx, request, facts, prepared, func(txCtx context.Context, response fosite.AccessResponder) error {
				return provider.ccHandler.PopulateTokenEndpointResponse(txCtx, request, response)
			})
			if change == "selection-changed" {
				require.ErrorContains(t, err, "signing key selection changed")
				require.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, response.AccessToken)
			}
			require.Equal(t, 1, encryptor.DecryptCalls())
			events, err := recorder.Query(ctx, model.BusinessEventQuery{Subject: model.BusinessEventSubject{NoSubject: true}, Type: model.BusinessEventTypePrefix + "token-issued", Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Hour)})
			require.NoError(t, err)
			if change == "selection-changed" {
				require.Empty(t, events)
			} else {
				require.Len(t, events, 1)
			}
		})
	}
}

func TestProviderPKCERejectionStillConsumesChallengeWhenSignerPreparationFails(t *testing.T) {
	provider, agents, service := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	ctx := context.Background()
	redirect, verifier := "http://localhost:8080/callback", "signer-failure-verifier-123456789012345678901234567890"
	agent.RedirectURIs = []string{redirect}
	require.NoError(t, agents.Update(ctx, agent))
	code, err := provider.HandleAuthorize(ctx, agent.ID.String(), redirect, "code", "read", "state", generateS256Challenge(verifier), "S256", id.Principal("pkce-signer-failure"))
	require.NoError(t, err)
	service.encryption = &failingDecryptor{}
	service.invalidateSigner()
	response, err := provider.HandleAuthorizationCodeExchange(ctx, agent.ID.String(), secret, code, redirect, "wrong-verifier-12345678901234567890123456789012")
	require.ErrorIs(t, err, ErrInvalidGrant)
	require.Nil(t, response)
	_, err = provider.fositeStorage.pkceRepo.FindBySignature(ctx, provider.authCodeHandler.AuthorizeCodeStrategy.AuthorizeCodeSignature(ctx, code))
	require.True(t, ports.IsNotFoundErr(err), "the invalid verifier still consumes its one-shot challenge")
}
