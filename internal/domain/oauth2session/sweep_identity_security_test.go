package oauth2session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type sweepIdentityObservations struct {
	ports.ThirdpartyOAuth2ClientIdentityRepository
	calls int
}

func (r *sweepIdentityObservations) RecordObservedClientIdentity(ctx context.Context, serviceID id.ServiceID, observed id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
	r.calls++
	return r.ThirdpartyOAuth2ClientIdentityRepository.RecordObservedClientIdentity(ctx, serviceID, observed, expected)
}

func TestClientIdentitySweepCountsSupersededSessionsAsFailedWithoutTokenWrites(t *testing.T) {
	for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		for _, dryRun := range []bool{false, true} {
			for _, rollback := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/dry=%t/rollback=%t", source, dryRun, rollback), func(t *testing.T) {
					reader := sdkmetric.NewManualReader()
					meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
					previous := otel.GetMeterProvider()
					otel.SetMeterProvider(meter)
					t.Cleanup(func() { otel.SetMeterProvider(previous); assert.NoError(t, meter.Shutdown(context.Background())) })
					binding := credentialPair(t, "sweep-old-identity-sentinel", "sweep-secret-sentinel")
					logs := new(strings.Builder)
					logger := slog.New(slog.NewJSONHandler(logs, nil))
					h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"sweep-identity": binding}, logger)
					clientID, secret := "sweep-old-identity-sentinel", "sweep-secret-sentinel"
					if source == model.CredentialSourceFilesystem {
						clientID, secret = "", ""
					}
					provider := h.register(t, source, "sweep-identity", clientID, secret)
					if source == model.CredentialSourceFilesystem {
						_, applied, _, err := h.records.RecordObservedClientIdentity(context.Background(), provider.ID, "sweep-old-identity-sentinel", ports.ClientIdentityObservation{ServiceVersion: provider.Version, CanonicalID: "sweep-identity"})
						require.NoError(t, err)
						require.True(t, applied)
					}
					established := id.ClientID("sweep-old-identity-sentinel")
					session := h.seedSession(t, "sweep-owner@example.com", provider.ID, &established)
					if source == model.CredentialSourceStored {
						h.transition(t, provider.ID, source, "sweep-new-identity-sentinel", "sweep-secret-sentinel")
						if rollback {
							h.transition(t, provider.ID, source, "sweep-old-identity-sentinel", "sweep-secret-sentinel")
						}
					} else if rollback {
						current, err := h.records.Get(context.Background(), provider.ID)
						require.NoError(t, err)
						_, applied, changed, err := h.records.RecordObservedClientIdentity(context.Background(), provider.ID, "sweep-new-identity-sentinel", ports.ClientIdentityObservation{ServiceVersion: current.Version, CanonicalID: "sweep-identity", Revision: current.ClientIdentityRevision})
						require.NoError(t, err)
						require.True(t, applied)
						require.True(t, changed)
					} else {
						require.NoError(t, publishCredential(binding.ClientIDFile, "sweep-new-identity-sentinel"))
					}
					before, err := h.records.Get(context.Background(), provider.ID)
					require.NoError(t, err)
					observations := &sweepIdentityObservations{ThirdpartyOAuth2ClientIdentityRepository: h.records}
					h.service.WithClientIdentityRepository(observations)
					logs.Reset()
					result, err := oauth2session.NewSessionSweepService(h.sessions.(ports.UserSessionExpiryRepository), h.service, logger).Sweep(context.Background(), oauth2session.SweepRequest{DryRun: dryRun})
					require.NoError(t, err)
					assertSweepCounts(t, result, 0, 0, 1, dryRun)
					assert.Empty(t, h.endpoint.captured(t))
					persisted, err := h.sessions.Get(context.Background(), session.ID)
					require.NoError(t, err)
					assert.Equal(t, session, persisted, "a denied candidate never rewrites tokens, timestamps, rejection, or establishment evidence")
					after, err := h.records.Get(context.Background(), provider.ID)
					require.NoError(t, err)
					if dryRun {
						assert.Zero(t, observations.calls, "dry run must not even attempt an observation write")
						assert.Equal(t, before, after)
						assert.Empty(t, sweepMetrics(t, reader), "dry-run classification changes no refresh counter")
					} else if source == model.CredentialSourceFilesystem {
						assert.Equal(t, before.ClientIdentityRevision+1, after.ClientIdentityRevision)
					}
					reason := model.CredentialSourceReasonIdentityMismatch
					if rollback {
						reason = model.CredentialSourceReasonIdentitySuperseded
					}
					assertCredentialSourceAudits(t, logs.String(), provider.ID, "refresh", credentialSourceAudit{event: "session.oauth2.credential_source_failed", outcome: "failed", reason: string(reason)})
					assert.NotContains(t, logs.String(), `"event":"session.oauth2.refresh_failed"`)
					changes, failures := 0, 0
					for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
						if line == "" {
							continue
						}
						var record map[string]any
						require.NoError(t, json.Unmarshal([]byte(line), &record))
						switch record["event"] {
						case "session.oauth2.client_identity_changed":
							changes++
						case "session.oauth2.credential_source_failed":
							failures++
							assert.Equal(t, "WARN", record["level"])
						}
					}
					assert.Equal(t, 1, failures)
					if dryRun || source == model.CredentialSourceStored {
						assert.Zero(t, changes)
					} else {
						assert.Equal(t, 1, changes)
					}
					for _, sensitive := range []string{"sweep-old-identity-sentinel", "sweep-new-identity-sentinel", "sweep-secret-sentinel", binding.ClientIDFile, binding.ClientSecretFile, "legacy-access", "legacy-refresh"} {
						assert.NotContains(t, logs.String(), sensitive)
					}
				})
			}
		}
	}
}
