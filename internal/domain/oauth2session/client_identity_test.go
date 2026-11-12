package oauth2session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func identityPointer(value string) *id.ClientID { result := id.ClientID(value); return &result }

func TestSessionIdentityStatus(t *testing.T) {
	for _, variant := range []string{"confidential", "public", "cimd", "google", "dcr", "filesystem"} {
		for _, tc := range []struct {
			name                      string
			established               *id.ClientID
			revision, currentRevision int64
			transitioned              bool
			reason                    model.CredentialSourceReason
		}{
			{name: "match", established: identityPointer("current"), revision: 3, currentRevision: 3},
			{name: "mismatch", established: identityPointer("previous"), currentRevision: 3, reason: model.CredentialSourceReasonIdentityMismatch},
			{name: "superseded", established: identityPointer("current"), revision: 1, currentRevision: 3, reason: "identity_superseded"},
			{name: "legacy"},
			{name: "legacy after change", currentRevision: 1, reason: model.CredentialSourceReasonIdentityMissing},
			{name: "legacy after transition", transitioned: true, reason: model.CredentialSourceReasonIdentityMissing},
			{name: "empty", established: identityPointer(""), reason: model.CredentialSourceReasonIdentityMissing},
		} {
			t.Run(variant+"/"+tc.name, func(t *testing.T) {
				service := &model.ThirdpartyOAuth2ProviderEntity{CredentialSource: model.CredentialSourceStored, CredentialSourceTransitioned: tc.transitioned}
				switch variant {
				case "public":
					service.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
				case "cimd":
					service.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodPrivateKeyJWT
				case "google":
					service.Flavor = model.OAuth2FlavorGoogle
				case "dcr":
					service.Discovery.ClientMethod = model.ClientBootstrapDCR
				case "filesystem":
					service.CredentialSource = model.CredentialSourceFilesystem
				}
				reason := tc.reason
				if variant == "filesystem" && tc.established == nil {
					reason = model.CredentialSourceReasonIdentityMissing
				}
				err := sessionIdentityStatus(&storage.UserSession{UpstreamClientID: tc.established, UpstreamClientRevision: tc.revision}, service, ports.ClientIdentityState{Identity: identityPointer("current"), Revision: tc.currentRevision})
				if reason == "" {
					require.NoError(t, err)
				} else {
					var source *model.CredentialSourceError
					require.ErrorAs(t, err, &source)
					assert.Equal(t, reason, source.Reason)
				}
			})
		}
	}
}

type identityProviderRepository struct {
	ports.ThirdpartyOAuth2ProviderRepository
	get func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)
}

func (r identityProviderRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return r.get(ctx, serviceID)
}

type identityEncryption struct{ ports.EncryptionPort }
type identityBranchKeys struct{ ports.BranchKeyManager }
type identityObservationRepository struct {
	observe func(context.Context, id.ServiceID, id.ClientID, ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error)
}

func (r identityObservationRepository) RecordObservedClientIdentity(ctx context.Context, serviceID id.ServiceID, value id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
	return r.observe(ctx, serviceID, value, expected)
}

type identityReader struct{ read func(string) (string, error) }

func (r identityReader) ReadClientID(path string) (string, error) { return r.read(path) }
func (r identityReader) ReadPair(string, string) (string, string, error) {
	panic("identity evaluation must not read a secret")
}

func identityService(t *testing.T, get func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)) (*OAuth2SessionService, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	providers := thirdparty.NewThirdpartyOAuth2ProviderService(identityProviderRepository{get: get}, identityEncryption{}, identityBranchKeys{}, nil, false, logger)
	return &OAuth2SessionService{providerService: providers, logger: logger, config: Config{CredentialFiles: map[string]ports.CredentialFileBinding{"first": {ClientIDFile: "first-file"}, "second": {ClientIDFile: "second-file"}}}}, &logs
}

func TestResolveClientIdentityDiscardLostObservation(t *testing.T) {
	for _, target := range []string{"stored", "new binding", "same binding new generation", "source round trip"} {
		t.Run(target, func(t *testing.T) {
			serviceID := id.NewServiceID()
			canonical := "first"
			initial := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, Version: 2, CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: identityPointer("old"), ClientIdentityRevision: 4}
			current := initial.Copy()
			current.Version = 3
			switch target {
			case "source round trip":
				current.Version = 4
				current.CredentialSourceTransitioned = true
			case "stored":
				current.CredentialSource = model.CredentialSourceStored
				current.ClientID = "fresh"
				current.CredentialSourceTransitioned = true
			case "new binding":
				value := "second"
				current.CanonicalID = &value
			}
			gets, reads, observations := 0, 0, 0
			service, logs := identityService(t, func(_ context.Context, got id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				gets++
				require.Equal(t, serviceID, got)
				return current.Copy(), nil
			})
			service.credentialFileReader = identityReader{read: func(path string) (string, error) {
				reads++
				if reads == 1 {
					assert.Equal(t, "first-file", path)
					return "obsolete", nil
				}
				if target == "new binding" {
					assert.Equal(t, "second-file", path)
				}
				return "fresh", nil
			}}
			service.WithClientIdentityRepository(identityObservationRepository{observe: func(_ context.Context, got id.ServiceID, observed id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
				observations++
				require.Equal(t, serviceID, got)
				if observations == 1 {
					assert.Equal(t, id.ClientID("obsolete"), observed)
					assert.Equal(t, int64(2), expected.ServiceVersion)
					return ports.ClientIdentityState{}, false, false, nil
				}
				assert.Equal(t, id.ClientID("fresh"), observed)
				assert.Equal(t, current.Version, expected.ServiceVersion)
				assert.Equal(t, *current.CanonicalID, expected.CanonicalID)
				assert.Equal(t, int64(4), expected.Revision)
				return ports.ClientIdentityState{Identity: identityPointer("fresh"), Revision: 5}, true, true, nil
			}})
			state, err := service.resolveClientIdentity(context.Background(), initial)
			require.NoError(t, err)
			require.NotNil(t, state.Identity)
			assert.Equal(t, id.ClientID("fresh"), *state.Identity)
			assert.Equal(t, 1, gets)
			assert.Equal(t, current.Version, initial.Version)
			if target == "stored" {
				assert.Equal(t, 1, reads)
				assert.Equal(t, 1, observations)
				assert.Equal(t, int64(4), state.Revision)
				assert.NotContains(t, logs.String(), "session.oauth2.client_identity_changed")
			} else {
				assert.Equal(t, 2, reads)
				assert.Equal(t, 2, observations)
				assert.Equal(t, int64(5), state.Revision)
				assert.Contains(t, logs.String(), `"channel":"filesystem_observation"`)
				assert.Contains(t, logs.String(), `"client_identity_revision":5`)
			}
			for _, forbidden := range []string{"obsolete", "fresh", "first-file", "second-file"} {
				assert.NotContains(t, logs.String(), forbidden)
			}
		})
	}
}

func TestIdentityEvaluatorComposesReauthentication(t *testing.T) {
	for _, expired := range []bool{false, true} {
		for _, refresh := range []bool{false, true} {
			for _, rejected := range []bool{false, true} {
				for _, superseded := range []bool{false, true} {
					t.Run(fmt.Sprintf("expired=%t/refresh=%t/rejected=%t/superseded=%t", expired, refresh, rejected, superseded), func(t *testing.T) {
						serviceID := id.NewServiceID()
						service, _ := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
							return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, ClientID: "current", CredentialSource: model.CredentialSourceStored, ClientIdentityRevision: 2}, nil
						})
						session := &storage.UserSession{ServiceID: serviceID, UpstreamClientID: identityPointer("current"), UpstreamClientRevision: 2}
						if superseded {
							session.UpstreamClientRevision = 1
						}
						if expired {
							past := time.Now().Add(-time.Hour)
							session.AccessTokenExpiresAt = &past
						}
						if refresh {
							session.EncryptedRefreshToken = []byte("opaque")
						}
						if rejected {
							now := time.Now()
							session.RefreshRejectedAt = &now
						}
						got, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{session}, ports.UsabilityGate)
						require.NoError(t, err)
						assert.Equal(t, rejected || (expired && !refresh) || superseded, got[serviceID])
					})
				}
			}
		}
	}
}

func TestIdentityEvaluatorSourceFailures(t *testing.T) {
	for _, mode := range []ports.UsabilityMode{ports.UsabilityGate, ports.UsabilityDisplay} {
		for _, revision := range []int64{0, 1} {
			t.Run(fmt.Sprintf("mode=%d/revision=%d", mode, revision), func(t *testing.T) {
				serviceID := id.NewServiceID()
				canonical := "first"
				service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
					return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: identityPointer("current"), ClientIdentityRevision: revision}, nil
				})
				service.credentialFileReader = identityReader{read: func(string) (string, error) {
					return "", model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed)
				}}
				got, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{{ServiceID: serviceID, UpstreamClientID: identityPointer("current")}}, mode)
				if mode == ports.UsabilityGate {
					assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
				} else {
					require.NoError(t, err)
					assert.Equal(t, revision != 0, got[serviceID])
					assert.Contains(t, logs.String(), `"event":"session.oauth2.credential_source_failed"`)
				}
				assert.NotContains(t, logs.String(), "first-file")
				assert.NotContains(t, logs.String(), "current")
			})
		}
	}
}

func TestIdentityReadErrorsDoNotExposeDependencyDetails(t *testing.T) {
	for _, cause := range []error{
		errors.New("database failure includes sensitive-client-value"),
		fmt.Errorf("/sensitive/credential-path: %w", model.NewCredentialSourceError(model.CredentialSourceReasonPermissionDenied)),
		fmt.Errorf("sensitive-client-value: %w", context.Canceled),
		fmt.Errorf("sensitive-client-value: %w", context.DeadlineExceeded),
	} {
		err := identityReadError(cause)
		assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
		assert.NotContains(t, err.Error(), "sensitive")
		if errors.Is(cause, context.Canceled) {
			assert.ErrorIs(t, err, context.Canceled)
		}
		if errors.Is(cause, context.DeadlineExceeded) {
			assert.ErrorIs(t, err, context.DeadlineExceeded)
		}
	}
}

func TestResolveClientIdentityFailures(t *testing.T) {
	for _, problem := range []string{"missing binding", "missing reader", "missing repository", "repository error", "canceled", "contention", "invalid source", "negative revision"} {
		t.Run(problem, func(t *testing.T) {
			serviceID := id.NewServiceID()
			canonical := "first"
			entity := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, CredentialSource: model.CredentialSourceFilesystem}
			service, _ := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return entity.Copy(), nil
			})
			calls := 0
			service.credentialFileReader = identityReader{read: func(string) (string, error) { calls++; return "current", nil }}
			service.WithClientIdentityRepository(identityObservationRepository{observe: func(context.Context, id.ServiceID, id.ClientID, ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
				if problem == "repository error" {
					return ports.ClientIdentityState{}, false, false, errors.New("unavailable")
				}
				return ports.ClientIdentityState{}, false, false, nil
			}})
			ctx := context.Background()
			switch problem {
			case "invalid source":
				entity.CredentialSource = "unknown"
				entity.ClientID = "current"
			case "negative revision":
				entity.CredentialSource = model.CredentialSourceStored
				entity.ClientID = "current"
				entity.ClientIdentityRevision = -1
			case "missing binding":
				service.config.CredentialFiles = nil
			case "missing reader":
				service.credentialFileReader = nil
			case "missing repository":
				service.clientIdentityRepo = nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := service.resolveClientIdentity(ctx, entity)
			assert.Error(t, err)
			assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
			assert.LessOrEqual(t, calls, 8)
		})
	}
}
