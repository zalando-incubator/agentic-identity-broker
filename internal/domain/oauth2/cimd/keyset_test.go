package cimd

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type keySetTestFetcher struct {
	result *ports.CIMDFetchResult
	err    error
	calls  int
}

func (f *keySetTestFetcher) Fetch(_ context.Context, _ string) (*ports.CIMDFetchResult, error) {
	f.calls++
	return f.result, f.err
}

func keySetConfig() ports.CIMDClientAuthenticationConfig {
	return ports.CIMDClientAuthenticationConfig{
		KeySetCacheTTL:           time.Hour,
		KeySetMinRefetchInterval: time.Minute,
		KeySetMaxBytes:           65536,
		KeySetFetchTimeout:       5 * time.Second,
	}
}

func testKeySet(t *testing.T, keys ...jwk.Key) []byte {
	t.Helper()
	set := jwk.NewSet()
	for _, key := range keys {
		require.NoError(t, set.AddKey(key))
	}
	encoded, err := json.Marshal(set)
	require.NoError(t, err)
	return encoded
}

func testPublicRSAKey(t *testing.T, bits int, kid string) jwk.Key {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	require.NoError(t, err)
	key, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, kid))
	return key
}

func testPublicECKey(t *testing.T, curve elliptic.Curve, kid string) jwk.Key {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	require.NoError(t, err)
	key, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, kid))
	return key
}

func TestClientKeySetServiceInlineDoesNotFetch(t *testing.T) {
	inline, err := jwk.Parse(testKeySet(t, testPublicRSAKey(t, 2048, "inline-key")))
	require.NoError(t, err)
	fetcher := &keySetTestFetcher{}
	service := NewClientKeySetService(fetcher, keySetConfig(), time.Now, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", KeySet: inline}

	keys, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	assert.Equal(t, 1, keys.Len())
	assert.Equal(t, 0, fetcher.calls)
}

func TestClientKeySetServiceFetchesAndCachesJWKS(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	fetcher := &keySetTestFetcher{result: &ports.CIMDFetchResult{Body: testKeySet(t, testPublicRSAKey(t, 2048, "remote-key"))}}
	service := NewClientKeySetService(fetcher, keySetConfig(), func() time.Time { return now }, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}

	first, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	second, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	assert.Equal(t, 1, first.Len())
	assert.Equal(t, 1, second.Len())
	assert.Equal(t, 1, fetcher.calls)

	now = now.Add(2 * time.Hour)
	_, err = service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	assert.Equal(t, 2, fetcher.calls, "expired key sets must be fetched again")
}

func TestClientKeySetServiceAppliesCurrentSigningAlgorithmToCachedKeys(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	rsaKey := testPublicRSAKey(t, 2048, "rsa-key")
	ecKey := testPublicECKey(t, elliptic.P256(), "ec-key")
	fetcher := &keySetTestFetcher{result: &ports.CIMDFetchResult{Body: testKeySet(t, rsaKey, ecKey)}}
	service := NewClientKeySetService(fetcher, keySetConfig(), func() time.Time { return now }, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}

	rsaKeys, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	if _, found := rsaKeys.LookupKeyID("rsa-key"); !found {
		t.Fatal("RS256 key was not returned")
	}
	if _, found := rsaKeys.LookupKeyID("ec-key"); found {
		t.Fatal("ES256 key was returned for an RS256 document")
	}

	doc.SigningAlg = "ES256"
	ecKeys, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	if _, found := ecKeys.LookupKeyID("ec-key"); !found {
		t.Fatal("cached ES256 key was not selected after the document algorithm changed")
	}
	if _, found := ecKeys.LookupKeyID("rsa-key"); found {
		t.Fatal("cached RSA key was returned for an ES256 document")
	}
	assert.Equal(t, 1, fetcher.calls, "algorithm changes must not require another fetch")
}

func TestClientKeySetServiceBoundsCacheEntries(t *testing.T) {
	fetcher := &keySetTestFetcher{result: &ports.CIMDFetchResult{Body: testKeySet(t, testPublicRSAKey(t, 2048, "shared-key"))}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := NewClientKeySetService(fetcher, keySetConfig(), time.Now, logger)

	for index := range 1001 {
		doc := &ports.CIMDMetadataDTO{
			ClientID:   "https://client.example/" + strconv.Itoa(index),
			AuthMethod: "private_key_jwt",
			SigningAlg: "RS256",
			JWKSURI:    "https://keys.example/" + strconv.Itoa(index) + ".json",
		}
		_, err := service.KeysFor(context.Background(), doc)
		require.NoError(t, err)
	}

	assert.LessOrEqual(t, len(service.cache), 1000)
}

func TestClientKeySetServiceFiltersIneligibleKeys(t *testing.T) {
	strongRSA := testPublicRSAKey(t, 2048, "strong-rsa")
	encRSA := testPublicRSAKey(t, 2048, "encryption-only")
	require.NoError(t, encRSA.Set(jwk.KeyUsageKey, "enc"))
	opsRSA := testPublicRSAKey(t, 2048, "no-verify-op")
	require.NoError(t, opsRSA.Set(jwk.KeyOpsKey, []string{"encrypt"}))
	p384 := testPublicECKey(t, elliptic.P384(), "wrong-curve")
	privateRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privateJWK, err := jwk.Import[jwk.Key](privateRSA)
	require.NoError(t, err)
	require.NoError(t, privateJWK.Set(jwk.KeyIDKey, "private-rsa"))

	body := testKeySet(t, strongRSA, encRSA, opsRSA, p384, privateJWK)
	var keySetDocument map[string][]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &keySetDocument))
	weakModulus := make([]byte, 128)
	weakModulus[0] = 0x80
	weakKey, err := json.Marshal(map[string]string{
		"kty": "RSA",
		"kid": "weak-rsa",
		"n":   base64.RawURLEncoding.EncodeToString(weakModulus),
		"e":   base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
	})
	require.NoError(t, err)
	keySetDocument["keys"] = append(keySetDocument["keys"], weakKey)
	body, err = json.Marshal(keySetDocument)
	require.NoError(t, err)
	fetcher := &keySetTestFetcher{result: &ports.CIMDFetchResult{Body: body}}
	service := NewClientKeySetService(fetcher, keySetConfig(), time.Now, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}

	keys, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	assert.Equal(t, 1, keys.Len())
	if _, ok := keys.LookupKeyID("strong-rsa"); !ok {
		t.Fatal("eligible RSA signing key was dropped")
	}
	for _, rejectedKID := range []string{"weak-rsa", "encryption-only", "no-verify-op", "wrong-curve", "private-rsa"} {
		if _, ok := keys.LookupKeyID(rejectedKID); ok {
			t.Errorf("ineligible key %q was retained", rejectedKID)
		}
	}

	esKey := testPublicECKey(t, elliptic.P256(), "p256-key")
	esFetcher := &keySetTestFetcher{result: &ports.CIMDFetchResult{Body: testKeySet(t, esKey)}}
	esService := NewClientKeySetService(esFetcher, keySetConfig(), time.Now, slog.Default())
	esDoc := &ports.CIMDMetadataDTO{ClientID: "https://ec.example/client", AuthMethod: "private_key_jwt", SigningAlg: "ES256", JWKSURI: "https://ec.example/jwks.json"}
	esKeys, err := esService.KeysFor(context.Background(), esDoc)
	require.NoError(t, err)
	assert.Equal(t, 1, esKeys.Len())
}

func TestClientKeySetServiceFailsClosedAndPreservesOldCache(t *testing.T) {
	now := time.Now().UTC()
	goodBody := testKeySet(t, testPublicRSAKey(t, 2048, "cached-key"))
	fetcher := &keySetTestFetcher{result: &ports.CIMDFetchResult{Body: goodBody}}
	service := NewClientKeySetService(fetcher, keySetConfig(), func() time.Time { return now }, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}
	first, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	require.Equal(t, 1, first.Len())

	now = now.Add(2 * time.Hour)
	for _, failure := range []struct {
		name string
		set  *ports.CIMDFetchResult
		err  error
	}{
		{name: "network", err: errors.New("connection refused")},
		{name: "oversized", set: &ports.CIMDFetchResult{Body: make([]byte, keySetConfig().KeySetMaxBytes+1)}},
		{name: "malformed", set: &ports.CIMDFetchResult{Body: []byte("{")}},
	} {
		t.Run(failure.name, func(t *testing.T) {
			fetcher.result = failure.set
			fetcher.err = failure.err
			_, err := service.KeysFor(context.Background(), doc)
			require.ErrorIs(t, err, ErrKeySetUnavailable)
			entry := service.cache[clientKeySetCacheKey{clientID: doc.ClientID, source: doc.JWKSURI}]
			require.NotNil(t, entry)
			assert.Equal(t, 1, entry.keys.Len(), "failed refetch must leave previous key set untouched")
		})
	}
}

func TestClientKeySetServiceRefreshForKIDRespectsIntervalAndReplacesRemovedKeys(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	oldKey := testPublicRSAKey(t, 2048, "old-key")
	newKey := testPublicRSAKey(t, 2048, "new-key")
	fetcher := &keySetTestFetcher{result: &ports.CIMDFetchResult{Body: testKeySet(t, oldKey)}}
	service := NewClientKeySetService(fetcher, keySetConfig(), func() time.Time { return now }, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}
	initial, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	assert.Equal(t, 1, initial.Len())

	fetcher.result = &ports.CIMDFetchResult{Body: testKeySet(t, newKey)}
	now = now.Add(keySetConfig().KeySetMinRefetchInterval - time.Nanosecond)
	withinInterval, err := service.RefreshForKID(context.Background(), doc, "new-key")
	require.NoError(t, err)
	assert.Equal(t, 1, fetcher.calls)
	if _, found := withinInterval.LookupKeyID("old-key"); !found {
		t.Fatal("refresh before the minimum interval must retain the cached key set")
	}

	now = now.Add(time.Nanosecond)
	refreshed, err := service.RefreshForKID(context.Background(), doc, "new-key")
	require.NoError(t, err)
	assert.Equal(t, 2, fetcher.calls)
	if _, found := refreshed.LookupKeyID("new-key"); !found {
		t.Fatal("new key was not returned after a successful refresh")
	}
	if _, found := refreshed.LookupKeyID("old-key"); found {
		t.Fatal("a key removed upstream must not survive a successful refresh")
	}
}

type concurrentKeySetFetcher struct {
	calls          atomic.Int32
	initial        *ports.CIMDFetchResult
	refreshed      *ports.CIMDFetchResult
	refreshStarted chan struct{}
	releaseRefresh chan struct{}
	startOnce      sync.Once
}

func (f *concurrentKeySetFetcher) Fetch(context.Context, string) (*ports.CIMDFetchResult, error) {
	if f.calls.Add(1) == 1 {
		return f.initial, nil
	}
	f.startOnce.Do(func() { close(f.refreshStarted) })
	<-f.releaseRefresh
	return f.refreshed, nil
}

func TestClientKeySetServiceRefreshForKIDDeduplicatesConcurrentCallers(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	initial := &ports.CIMDFetchResult{Body: testKeySet(t, testPublicRSAKey(t, 2048, "old-key"))}
	refreshed := &ports.CIMDFetchResult{Body: testKeySet(t, testPublicRSAKey(t, 2048, "new-key"))}
	fetcher := &concurrentKeySetFetcher{
		initial:        initial,
		refreshed:      refreshed,
		refreshStarted: make(chan struct{}),
		releaseRefresh: make(chan struct{}),
	}
	service := NewClientKeySetService(fetcher, keySetConfig(), func() time.Time { return now }, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}
	_, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	now = now.Add(keySetConfig().KeySetMinRefetchInterval)

	const callers = 20
	var wait sync.WaitGroup
	errorsFound := make(chan error, callers)
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := service.RefreshForKID(context.Background(), doc, "new-key")
			errorsFound <- err
		}()
	}
	<-fetcher.refreshStarted
	close(fetcher.releaseRefresh)
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(2), fetcher.calls.Load(), "one initial fetch plus one shared refresh is expected")
}

type concurrentColdKeySetFetcher struct {
	calls              atomic.Int32
	result             *ports.CIMDFetchResult
	firstFetchStarted  chan struct{}
	secondFetchStarted chan struct{}
	releaseFetches     chan struct{}
}

func (f *concurrentColdKeySetFetcher) Fetch(context.Context, string) (*ports.CIMDFetchResult, error) {
	switch f.calls.Add(1) {
	case 1:
		close(f.firstFetchStarted)
	case 2:
		close(f.secondFetchStarted)
	}
	<-f.releaseFetches
	return f.result, nil
}

func TestClientKeySetServiceDeduplicatesConcurrentColdFetches(t *testing.T) {
	fetcher := &concurrentColdKeySetFetcher{
		result:             &ports.CIMDFetchResult{Body: testKeySet(t, testPublicRSAKey(t, 2048, "cold-key"))},
		firstFetchStarted:  make(chan struct{}),
		secondFetchStarted: make(chan struct{}),
		releaseFetches:     make(chan struct{}),
	}
	service := NewClientKeySetService(fetcher, keySetConfig(), time.Now, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}

	const callers = 20
	ready := make(chan struct{}, callers)
	start := make(chan struct{})
	errorsFound := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			keys, err := service.KeysFor(context.Background(), doc)
			if err == nil && keys.Len() != 1 {
				err = errors.New("cold fetch returned an unexpected key set")
			}
			errorsFound <- err
		}()
	}
	for range callers {
		<-ready
	}
	close(start)
	<-fetcher.firstFetchStarted

	select {
	case <-fetcher.secondFetchStarted:
		close(fetcher.releaseFetches)
		wait.Wait()
		t.Fatal("concurrent cold misses started duplicate key-set fetches")
	case <-time.After(100 * time.Millisecond):
	}
	close(fetcher.releaseFetches)
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), fetcher.calls.Load())
}

func TestClientKeySetServiceRefreshForKIDFailurePreservesCache(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	fetcher := &keySetTestFetcher{result: &ports.CIMDFetchResult{Body: testKeySet(t, testPublicRSAKey(t, 2048, "cached-key"))}}
	service := NewClientKeySetService(fetcher, keySetConfig(), func() time.Time { return now }, slog.Default())
	doc := &ports.CIMDMetadataDTO{ClientID: "https://client.example/metadata", AuthMethod: "private_key_jwt", SigningAlg: "RS256", JWKSURI: "https://client.example/jwks.json"}
	_, err := service.KeysFor(context.Background(), doc)
	require.NoError(t, err)
	now = now.Add(keySetConfig().KeySetMinRefetchInterval)
	fetcher.err = errors.New("connection refused")

	_, err = service.RefreshForKID(context.Background(), doc, "unknown-key")
	require.ErrorIs(t, err, ErrKeySetUnavailable)
	entry := service.cache[clientKeySetCacheKey{clientID: doc.ClientID, source: doc.JWKSURI}]
	require.NotNil(t, entry)
	if _, found := entry.keys.LookupKeyID("cached-key"); !found {
		t.Fatal("failed refresh must preserve the previously cached key set")
	}
}
