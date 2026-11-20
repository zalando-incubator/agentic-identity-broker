package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/bizevent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var builderBusinessEvent = bizevent.MustDefine("test.builder.recorded")

func businessEventsBuilderConfig(enabled bool) *ports.Config {
	return &ports.Config{
		Server:           ports.ServerConfig{EndUser: ports.ServerInstanceConfig{PublicURL: "http://localhost:8000"}},
		Storage:          ports.StorageConfig{Backend: "memory", Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second}},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))},
		Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{Mode: servermode.Local, Local: ports.LocalModeConfig{TokenTTL: time.Hour}},
		BusinessEvents:   ports.BusinessEventsConfig{Enabled: enabled, WriteTimeout: time.Second, Retention: ports.BusinessEventsRetentionConfig{Period: 24 * time.Hour, Interval: time.Second, BatchSize: 1000}},
	}
}

func TestBuilderBusinessEventPipeline(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			cfg := businessEventsBuilderConfig(enabled)
			store, err := storage.NewAdapter(&cfg.Storage)
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, store.Close(context.Background())) })
			var output bytes.Buffer
			application, err := NewBuilder().WithConfig(cfg).WithStorage(store).
				WithLogger(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelError}))).
				WithJWKSPublisher(&mockJWKSPublisher{}).Build()
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, application.Shutdown(context.Background())) })
			application.Logger.InfoContext(t.Context(), "builder event", "event", builderBusinessEvent)
			deleted, err := store.BusinessEventRetention().DeleteRecordedBefore(t.Context(), time.Now().Add(time.Hour), 100)
			require.NoError(t, err)
			want := int64(0)
			if enabled {
				want = 1
			}
			assert.Equal(t, want, deleted, "typed Info records must reach the enabled ledger independently of the operational level")
			assert.Empty(t, output.String())
		})
	}
}

func TestBuilderBusinessEventsRequiresAppender(t *testing.T) {
	application, err := NewBuilder().WithConfig(businessEventsBuilderConfig(true)).
		WithStorage(&storage.Adapter{}).WithLogger(slog.New(slog.DiscardHandler)).Build()
	require.Error(t, err)
	assert.Nil(t, application)
}

func TestBuilderBusinessEventRetentionLifecycle(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cfg := businessEventsBuilderConfig(enabled)
				store, err := storage.NewAdapter(&cfg.Storage)
				require.NoError(t, err)
				defer func() { assert.NoError(t, store.Close(context.Background())) }()
				seed := func() {
					require.NoError(t, store.BusinessEvents().Append(t.Context(), &model.BusinessEvent{ID: id.NewBusinessEventID(), Event: "test.builder.recorded", OccurredAt: time.Now()}))
				}
				seed()
				time.Sleep(25 * time.Hour)
				seed()
				application, err := NewBuilder().WithConfig(cfg).WithStorage(store).WithLogger(slog.New(slog.DiscardHandler)).WithJWKSPublisher(&mockJWKSPublisher{}).Build()
				require.NoError(t, err)
				defer func() { assert.NoError(t, application.Shutdown(context.Background())) }()
				synctest.Wait()
				count, err := store.BusinessEventRetention().DeleteRecordedBefore(t.Context(), time.Now().Add(-24*time.Hour), 100)
				require.NoError(t, err)
				want := int64(1)
				if enabled {
					want = 0
				}
				assert.Equal(t, want, count, "only an enabled ledger runs startup cleanup")
				count, err = store.BusinessEventRetention().DeleteRecordedBefore(t.Context(), time.Now().Add(time.Hour), 100)
				require.NoError(t, err)
				assert.Equal(t, int64(1), count, "startup cleanup preserves the fresh row")
				require.NoError(t, application.Shutdown(context.Background()))
				seed()
				time.Sleep(25 * time.Hour)
				synctest.Wait()
				count, err = store.BusinessEventRetention().DeleteRecordedBefore(t.Context(), time.Now().Add(-24*time.Hour), 100)
				require.NoError(t, err)
				assert.Equal(t, int64(1), count, "shutdown must stop retention")
			})
		})
	}
}
