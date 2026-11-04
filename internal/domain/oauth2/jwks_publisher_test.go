package oauth2

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// --- hand-rolled mocks ---

type mockSigningKeyManager struct {
	buildFn func(ctx context.Context) (jwk.Set, error)
}

func (m *mockSigningKeyManager) GenerateAndStoreKey(_ context.Context, _ string, _ bool) (*storage.SigningKey, error) {
	panic("not implemented")
}

func (m *mockSigningKeyManager) BuildJWKS(ctx context.Context) (jwk.Set, error) {
	return m.buildFn(ctx)
}

func (m *mockSigningKeyManager) ListKeys(_ context.Context) ([]*storage.SigningKey, error) {
	panic("not implemented")
}

func (m *mockSigningKeyManager) PromoteKey(_ context.Context, _ id.KeyID) (*storage.SigningKey, error) {
	panic("not implemented")
}

func (m *mockSigningKeyManager) DeleteKey(_ context.Context, _ id.KeyID) error {
	panic("not implemented")
}

type mockPublisherJWKSPort struct {
	getFn       func(ctx context.Context) (jwk.Set, error)
	healthState ports.ComponentHealth
}

func (m *mockPublisherJWKSPort) GetKeySet(ctx context.Context) (jwk.Set, error) {
	return m.getFn(ctx)
}

func (m *mockPublisherJWKSPort) GetKey(_ context.Context, _ string) (jwk.Key, error) {
	panic("not implemented")
}

func (m *mockPublisherJWKSPort) HealthState() ports.ComponentHealth {
	return m.healthState
}

var _ ports.SigningKeyManager = (*mockSigningKeyManager)(nil)
var _ ports.JWKSPort = (*mockPublisherJWKSPort)(nil)

// --- test helpers ---

func jwksTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func makePublicECKey(kid string) jwk.Key {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	raw, err := jwk.Import[jwk.Key](priv.Public())
	if err != nil {
		panic(err)
	}
	if kid != "" {
		if err := raw.Set(jwk.KeyIDKey, kid); err != nil {
			panic(err)
		}
	}
	if err := raw.Set(jwk.AlgorithmKey, jwa.ES256()); err != nil {
		panic(err)
	}
	return raw
}

func setOfKeys(keys ...jwk.Key) jwk.Set {
	s := jwk.NewSet()
	for _, k := range keys {
		if err := s.AddKey(k); err != nil {
			panic(err)
		}
	}
	return s
}

func keyFields(t *testing.T, key jwk.Key) map[string]any {
	t.Helper()

	payload, err := json.Marshal(key)
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(payload, &fields))
	return fields
}

// --- Constructor validation tests ---

func TestNewLocalJWKSPublisher_PanicsOnNilLocalKeys(t *testing.T) {
	assert.Panics(t, func() { NewLocalJWKSPublisher(nil, jwksTestLogger()) })
}

func TestNewLocalJWKSPublisher_PanicsOnNilLogger(t *testing.T) {
	localMock := &mockSigningKeyManager{buildFn: func(_ context.Context) (jwk.Set, error) { return jwk.NewSet(), nil }}
	assert.Panics(t, func() { NewLocalJWKSPublisher(localMock, nil) })
}

func TestNewProxyJWKSPublisher_PanicsOnNilLogger(t *testing.T) {
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return jwk.NewSet(), nil
		},
	}

	assert.PanicsWithValue(t, "BUG: NewProxyJWKSPublisher requires non-nil logger", func() {
		NewProxyJWKSPublisher(upstreamMock, nil)
	})
}

func TestNewProxyJWKSPublisher_PanicsOnNilUpstream(t *testing.T) {
	assert.Panics(t, func() { NewProxyJWKSPublisher(nil, jwksTestLogger()) })
}

func TestNewProxyJWKSPublisher_RequiresJWKSPortSignature(t *testing.T) {
	constructorType := reflect.TypeOf(NewProxyJWKSPublisher)
	upstreamParam := constructorType.In(0)
	jwksPortType := reflect.TypeOf((*ports.JWKSPort)(nil)).Elem()

	assert.True(t, upstreamParam.Implements(jwksPortType))
}

func TestNewHybridJWKSPublisher_PanicsOnNilLocalKeys(t *testing.T) {
	assert.Panics(t, func() { NewHybridJWKSPublisher(nil, nil, jwksTestLogger()) })
}

func TestNewHybridJWKSPublisher_PanicsOnNilLogger(t *testing.T) {
	localMock := &mockSigningKeyManager{buildFn: func(_ context.Context) (jwk.Set, error) { return jwk.NewSet(), nil }}
	assert.Panics(t, func() { NewHybridJWKSPublisher(localMock, nil, nil) })
}

func TestNewHybridJWKSPublisher_PanicsOnNilUpstream(t *testing.T) {
	localMock := &mockSigningKeyManager{buildFn: func(_ context.Context) (jwk.Set, error) { return jwk.NewSet(), nil }}
	assert.Panics(t, func() { NewHybridJWKSPublisher(localMock, nil, jwksTestLogger()) })
}

func TestNewHybridJWKSPublisher_RequiresJWKSPortSignature(t *testing.T) {
	constructorType := reflect.TypeOf(NewHybridJWKSPublisher)
	upstreamParam := constructorType.In(1)
	jwksPortType := reflect.TypeOf((*ports.JWKSPort)(nil)).Elem()

	assert.True(t, upstreamParam.Implements(jwksPortType))
}

// --- PublishJWKS tests ---

func TestPublishJWKS_Local_ReturnsOnlyLocalKeys(t *testing.T) {
	localKey := makePublicECKey("local-key-1")
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}

	svc := NewLocalJWKSPublisher(localMock, jwksTestLogger())
	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, set.Len())
	key, ok := set.LookupKeyID("local-key-1")
	assert.True(t, ok)
	kid, _ := key.KeyID()
	assert.Equal(t, "local-key-1", kid)
}

func TestPublishJWKS_Local_BuildJWKSFailure_ReturnsWrappedError(t *testing.T) {
	buildErr := errors.New("storage error")
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return nil, buildErr
		},
	}

	svc := NewLocalJWKSPublisher(localMock, jwksTestLogger())
	_, err := svc.PublishJWKS(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, buildErr)
	assert.ErrorContains(t, err, "failed to build local JWKS")
}

func TestPublishJWKS_Local_BuildJWKSFailure_LogsError(t *testing.T) {
	buildErr := errors.New("storage error")
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return nil, buildErr
		},
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))

	svc := NewLocalJWKSPublisher(localMock, logger)

	_, err := svc.PublishJWKS(context.Background())
	require.Error(t, err)
	assert.Contains(t, logs.String(), "failed to build local jwks")
	assert.Contains(t, logs.String(), buildErr.Error())
}

func TestPublishJWKS_Proxy_ReturnsOnlyUpstreamKeys(t *testing.T) {
	upstreamKey := makePublicECKey("upstream-key-1")
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(upstreamKey), nil
		},
	}

	svc := NewProxyJWKSPublisher(upstreamMock, jwksTestLogger())
	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, set.Len())
	key, ok := set.LookupKeyID("upstream-key-1")
	assert.True(t, ok)
	kid, _ := key.KeyID()
	assert.Equal(t, "upstream-key-1", kid)
}

func TestPublishJWKS_Proxy_EmptyUpstream_ReturnsEmptySet(t *testing.T) {
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return jwk.NewSet(), nil
		},
	}

	svc := NewProxyJWKSPublisher(upstreamMock, jwksTestLogger())
	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	require.NotNil(t, set)
	assert.Equal(t, 0, set.Len())
}

func TestPublishJWKS_Hybrid_ReturnsBothKeysets(t *testing.T) {
	localKey := makePublicECKey("local-key-1")
	upstreamKey := makePublicECKey("upstream-key-1")

	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(upstreamKey), nil
		},
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())
	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, set.Len())
	_, ok1 := set.LookupKeyID("local-key-1")
	_, ok2 := set.LookupKeyID("upstream-key-1")
	assert.True(t, ok1, "local key should be present")
	assert.True(t, ok2, "upstream key should be present")
}

func TestPublishJWKS_Hybrid_EmptyUpstream_ReturnsOnlyLocalKeys(t *testing.T) {
	localKey := makePublicECKey("local-key-1")
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return jwk.NewSet(), nil
		},
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())
	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, set.Len())
	_, ok := set.LookupKeyID("local-key-1")
	assert.True(t, ok)
}

func TestPublishJWKS_NeverExposesPrivateKeyMaterial(t *testing.T) {
	localKey := makePublicECKey("local-key-1")
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}

	tests := []struct {
		name string
		svc  *JWKSPublisherService
	}{
		{
			name: "local",
			svc:  NewLocalJWKSPublisher(localMock, jwksTestLogger()),
		},
		{
			name: "hybrid",
			svc: NewHybridJWKSPublisher(localMock, &mockPublisherJWKSPort{
				getFn: func(_ context.Context) (jwk.Set, error) {
					return setOfKeys(makePublicECKey("upstream-key-99")), nil
				},
			}, jwksTestLogger()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, err := tt.svc.PublishJWKS(context.Background())
			require.NoError(t, err)
			for i := 0; i < set.Len(); i++ {
				key, ok := set.Key(i)
				require.True(t, ok)
				kid, _ := key.KeyID()
				_, err := jwk.Get[any](key, "d")
				assert.Error(t, err, "key %q must not contain private 'd' field", kid)
			}
		})
	}
}

func TestPublishJWKS_Proxy_UpstreamError_ReturnsErrUpstreamUnavailable(t *testing.T) {
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return nil, errors.New("connection refused")
		},
	}

	svc := NewProxyJWKSPublisher(upstreamMock, jwksTestLogger())
	_, err := svc.PublishJWKS(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ports.ErrUpstreamUnavailable))
}

func TestPublishJWKS_Hybrid_UpstreamError_ReturnsErrUpstreamUnavailable(t *testing.T) {
	localKey := makePublicECKey("local-key-1")
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return nil, errors.New("upstream unreachable")
		},
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())
	_, err := svc.PublishJWKS(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ports.ErrUpstreamUnavailable))
}

func TestPublishJWKS_Hybrid_KidConflict_ReturnsErrKidConflict(t *testing.T) {
	conflictingKid := "shared-kid"
	localKey := makePublicECKey(conflictingKid)
	upstreamKey := makePublicECKey(conflictingKid)

	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(upstreamKey), nil
		},
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())
	_, err := svc.PublishJWKS(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ports.ErrKidConflict))
}

func TestPublishJWKS_Hybrid_NoKidConflict_PublishesAllKeys(t *testing.T) {
	localKey := makePublicECKey("local-1")
	upstreamKey := makePublicECKey("upstream-1")

	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(upstreamKey), nil
		},
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())
	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, set.Len())
}

func TestPublishJWKS_Hybrid_PreservesUpstreamKeyFields(t *testing.T) {
	localKey := makePublicECKey("local-1")
	upstreamKey := makePublicECKey("upstream-1")
	require.NoError(t, upstreamKey.Set(jwk.KeyUsageKey, "sig"))

	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(upstreamKey), nil
		},
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())
	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)

	publishedUpstreamKey, ok := set.LookupKeyID("upstream-1")
	require.True(t, ok)

	expectedFields := keyFields(t, upstreamKey)
	publishedFields := keyFields(t, publishedUpstreamKey)
	for _, field := range []string{"kid", "alg", "kty", "use", "crv", "x", "y"} {
		assert.Equalf(t, expectedFields[field], publishedFields[field], "field %s should be preserved", field)
	}
}

func TestPublishJWKS_Hybrid_KeysWithoutKidAreMerged(t *testing.T) {
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(makePublicECKey("")), nil
		},
	}
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(makePublicECKey("")), nil
		},
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())
	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, set.Len())
}

func TestPublishJWKS_Hybrid_RuntimeKidConflict_ReturnsErrKidConflict(t *testing.T) {
	localKey := makePublicECKey("shared-kid")

	callCount := 0
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			callCount++
			if callCount == 1 {
				return setOfKeys(makePublicECKey("upstream-original")), nil
			}
			return setOfKeys(makePublicECKey("shared-kid")), nil
		},
	}

	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(localKey), nil
		},
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())

	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, set.Len())

	_, err = svc.PublishJWKS(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ports.ErrKidConflict))
}

func TestPublishJWKS_Proxy_UpstreamError_LogsError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return nil, errors.New("upstream down")
		},
	}

	svc := NewProxyJWKSPublisher(upstreamMock, logger)

	_, err := svc.PublishJWKS(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ports.ErrUpstreamUnavailable))
	assert.Contains(t, logs.String(), "upstream jwks fetch failed")
}

func TestPublishJWKS_Proxy_UpstreamError_RelogsEveryTenFailures(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return nil, errors.New("upstream down")
		},
	}

	svc := NewProxyJWKSPublisher(upstreamMock, logger)

	for range 10 {
		_, err := svc.PublishJWKS(context.Background())
		require.Error(t, err)
	}

	assert.Equal(t, 2, bytes.Count(logs.Bytes(), []byte("upstream jwks fetch failed")))
}

func TestPublishJWKS_Proxy_Recovery_LogsInfo(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	calls := 0
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("upstream down")
			}
			return jwk.NewSet(), nil
		},
	}

	svc := NewProxyJWKSPublisher(upstreamMock, logger)

	_, err := svc.PublishJWKS(context.Background())
	require.Error(t, err)

	set, err := svc.PublishJWKS(context.Background())
	require.NoError(t, err)
	require.NotNil(t, set)
	assert.Contains(t, logs.String(), "upstream jwks fetch failed")
	assert.Contains(t, logs.String(), "upstream jwks fetch recovered")
}

// --- HealthState tests ---

func TestHealthState_Local_AlwaysHealthy(t *testing.T) {
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return jwk.NewSet(), nil
		},
	}
	svc := NewLocalJWKSPublisher(localMock, jwksTestLogger())
	assert.Equal(t, ports.ComponentHealthHealthy, svc.HealthState())
}

func TestHealthState_Proxy_DelegatesDegradedState(t *testing.T) {
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return jwk.NewSet(), nil
		},
		healthState: ports.ComponentHealthDegraded,
	}

	svc := NewProxyJWKSPublisher(upstreamMock, jwksTestLogger())
	assert.Equal(t, ports.ComponentHealthDegraded, svc.HealthState())
}

func TestHealthState_Proxy_DelegatesHealthyState(t *testing.T) {
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return jwk.NewSet(), nil
		},
		healthState: ports.ComponentHealthHealthy,
	}

	svc := NewProxyJWKSPublisher(upstreamMock, jwksTestLogger())
	assert.Equal(t, ports.ComponentHealthHealthy, svc.HealthState())
}

func TestHealthState_Hybrid_DelegatesDegradedState(t *testing.T) {
	localMock := &mockSigningKeyManager{
		buildFn: func(_ context.Context) (jwk.Set, error) {
			return setOfKeys(makePublicECKey("local-1")), nil
		},
	}
	upstreamMock := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return jwk.NewSet(), nil
		},
		healthState: ports.ComponentHealthDegraded,
	}

	svc := NewHybridJWKSPublisher(localMock, upstreamMock, jwksTestLogger())
	assert.Equal(t, ports.ComponentHealthDegraded, svc.HealthState())
}
