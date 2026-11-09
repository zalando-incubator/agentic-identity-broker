package cimd

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustNewCIMDCache(t *testing.T, minTTL, maxTTL time.Duration) *CIMDCache {
	t.Helper()
	c, err := NewCIMDCache(minTTL, maxTTL, 1000)
	require.NoError(t, err)
	return c
}

func TestCIMDCache(t *testing.T) {
	const testURL = "https://agent.example.com/client"
	doc := &ClientIDMetadataDocument{
		ClientID:     testURL,
		ClientName:   "Test Agent",
		RedirectURIs: []string{"https://agent.example.com/callback"},
	}

	minTTL := 60 * time.Second
	maxTTL := 1 * time.Hour

	t.Run("miss returns nil", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		assert.Nil(t, c.Get(testURL))
	})

	t.Run("hit returns entry within TTL", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		h := CacheHeaders{CacheControl: "max-age=300"}
		c.Set(testURL, doc, h, time.Now())

		entry := c.Get(testURL)
		require.NotNil(t, entry)
		assert.Equal(t, testURL, entry.URL)
		assert.Equal(t, doc, entry.Document)
	})

	t.Run("expired entry returns nil", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		h := CacheHeaders{CacheControl: "max-age=300"}
		fetchedAt := time.Now().Add(-10 * time.Minute) // 10 min ago
		c.Set(testURL, doc, h, fetchedAt)

		assert.Nil(t, c.Get(testURL))
	})

	t.Run("max-age from Cache-Control is used", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		h := CacheHeaders{CacheControl: "max-age=7200"} // 2 hours — clamped to maxTTL (1h)

		now := time.Now()
		c.Set(testURL, doc, h, now)

		entry := c.Get(testURL)
		require.NotNil(t, entry)
		expectedExpiry := now.Add(maxTTL)
		assert.WithinDuration(t, expectedExpiry, entry.ExpiresAt, time.Second)
	})

	t.Run("operator maxTTL clamps document max-age", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, 30*time.Minute)
		h := CacheHeaders{CacheControl: "max-age=7200"}

		now := time.Now()
		c.Set(testURL, doc, h, now)

		entry := c.Get(testURL)
		require.NotNil(t, entry)
		assert.WithinDuration(t, now.Add(30*time.Minute), entry.ExpiresAt, time.Second)
	})

	t.Run("minTTL floor applied when max-age is too short", func(t *testing.T) {
		c := mustNewCIMDCache(t, 60*time.Second, maxTTL)
		h := CacheHeaders{CacheControl: "max-age=10"} // 10s — below minTTL (60s)

		now := time.Now()
		c.Set(testURL, doc, h, now)

		entry := c.Get(testURL)
		require.NotNil(t, entry)
		assert.WithinDuration(t, now.Add(60*time.Second), entry.ExpiresAt, time.Second)
	})

	t.Run("minTTL applied when no Cache-Control header", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		now := time.Now()
		c.Set(testURL, doc, CacheHeaders{}, now)

		entry := c.Get(testURL)
		require.NotNil(t, entry)
		assert.WithinDuration(t, now.Add(minTTL), entry.ExpiresAt, time.Second)
	})

	t.Run("expired entry is evicted from map on Get", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		h := CacheHeaders{CacheControl: "max-age=300"}
		c.Set(testURL, doc, h, time.Now().Add(-10*time.Minute))

		assert.Nil(t, c.Get(testURL))

		c.mu.RLock()
		_, exists := c.entries[testURL]
		c.mu.RUnlock()
		assert.False(t, exists, "expired entry should be removed from map")
	})

	t.Run("Get does not evict entry refreshed by concurrent Set", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		h := CacheHeaders{CacheControl: "max-age=300"}

		freshDoc := &ClientIDMetadataDocument{ClientID: testURL, ClientName: "Fresh Agent"}

		// Seed expired, then immediately refresh (simulates concurrent Set winning the window).
		c.Set(testURL, doc, h, time.Now().Add(-10*time.Minute))
		c.Set(testURL, freshDoc, h, time.Now())

		entry := c.Get(testURL)
		require.NotNil(t, entry, "fresh entry must survive after concurrent Set")
		assert.Equal(t, "Fresh Agent", entry.Document.ClientName)
	})

	t.Run("Get concurrent with Set does not race", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		h := CacheHeaders{CacheControl: "max-age=300"}

		freshDoc := &ClientIDMetadataDocument{ClientID: testURL, ClientName: "Fresh"}
		c.Set(testURL, doc, h, time.Now().Add(-10*time.Minute))

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 200 {
				c.Set(testURL, freshDoc, h, time.Now())
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				_ = c.Get(testURL)
			}
		}()
		wg.Wait()
	})

	t.Run("Set deep-copies document so caller mutations do not corrupt cache", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		h := CacheHeaders{CacheControl: "max-age=300"}

		mutableDoc := &ClientIDMetadataDocument{
			ClientID:      testURL,
			ClientName:    "Legitimate Agent",
			RedirectURIs:  []string{"https://agent.example.com/callback"},
			GrantTypes:    []string{"authorization_code"},
			ResponseTypes: []string{"code"},
		}
		c.Set(testURL, mutableDoc, h, time.Now())

		// Mutate the original document's scalar and slice fields after Set.
		mutableDoc.ClientName = "Hijacked Agent"
		mutableDoc.RedirectURIs[0] = "https://evil.example.com/steal"
		mutableDoc.GrantTypes[0] = "implicit"
		mutableDoc.ResponseTypes[0] = "token"

		entry := c.Get(testURL)
		require.NotNil(t, entry)
		assert.Equal(t, "Legitimate Agent", entry.Document.ClientName)
		assert.Equal(t, "https://agent.example.com/callback", entry.Document.RedirectURIs[0])
		assert.Equal(t, "authorization_code", entry.Document.GrantTypes[0])
		assert.Equal(t, "code", entry.Document.ResponseTypes[0])
	})

	t.Run("Get returns an independent inline key set", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		keySet, err := jwk.Parse(publicTestJWKS(t))
		require.NoError(t, err)
		mutableDoc := &ClientIDMetadataDocument{
			ClientID:   testURL,
			AuthMethod: "private_key_jwt",
			SigningAlg: "RS256",
			JWKS:       json.RawMessage(publicTestJWKS(t)),
			KeySet:     keySet,
		}
		c.Set(testURL, mutableDoc, CacheHeaders{CacheControl: "max-age=300"}, time.Now())

		returned := c.Get(testURL)
		require.NotNil(t, returned)
		key, ok := returned.Document.KeySet.Key(0)
		require.True(t, ok)
		require.NoError(t, returned.Document.KeySet.RemoveKey(key))

		cached := c.Get(testURL)
		require.NotNil(t, cached)
		assert.Equal(t, 1, cached.Document.KeySet.Len(), "mutating a returned set must not alter the cache")
	})

	t.Run("rejects minTTL greater than maxTTL", func(t *testing.T) {
		_, err := NewCIMDCache(2*time.Hour, 30*time.Minute, 1000)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "minTTL")
	})

	t.Run("rejects non-positive maxEntries", func(t *testing.T) {
		_, err := NewCIMDCache(60*time.Second, time.Hour, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maxEntries")
	})

	t.Run("accepts equal minTTL and maxTTL", func(t *testing.T) {
		c, err := NewCIMDCache(5*time.Minute, 5*time.Minute, 1000)
		require.NoError(t, err)
		assert.NotNil(t, c)
	})

	t.Run("Set evicts oldest entry when maxEntries reached", func(t *testing.T) {
		c := mustNewCIMDCache(t, minTTL, maxTTL)
		c.maxEntries = 2

		h := CacheHeaders{CacheControl: "max-age=300"}

		c.Set("https://a.example.com/client", doc, h, time.Now().Add(-2*time.Second))
		c.Set("https://b.example.com/client", doc, h, time.Now().Add(-1*time.Second))
		c.Set("https://c.example.com/client", doc, h, time.Now()) // evicts oldest (a)

		c.mu.RLock()
		count := len(c.entries)
		c.mu.RUnlock()
		assert.Equal(t, 2, count, "cache must not exceed maxEntries")
		assert.Nil(t, c.Get("https://a.example.com/client"), "oldest entry must be evicted")
		assert.NotNil(t, c.Get("https://c.example.com/client"), "new entry must be cached")
	})
}
