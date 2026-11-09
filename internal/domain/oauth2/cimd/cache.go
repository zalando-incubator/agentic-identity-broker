package cimd

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CIMDCacheEntry holds a cached CIMD document with its expiry metadata.
type CIMDCacheEntry struct {
	URL       string
	Document  *ClientIDMetadataDocument
	FetchedAt time.Time
	ExpiresAt time.Time
}

// CacheHeaders holds the two HTTP response headers relevant to TTL derivation.
// Using a plain struct keeps the domain free of net/http.
type CacheHeaders struct {
	CacheControl string
	Expires      string
}

// httpTimeFormats mirrors the formats tried by net/http.ParseTime (IMF-fixdate, RFC850, ANSIC).
var httpTimeFormats = []string{
	"Mon, 02 Jan 2006 15:04:05 GMT",
	"Monday, 02-Jan-06 15:04:05 MST",
	"Mon Jan _2 15:04:05 2006",
}

func parseHTTPTime(s string) (time.Time, error) {
	for _, layout := range httpTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as HTTP-date", s)
}

// CIMDCache is a thread-safe in-process cache for CIMD documents.
// TTL is derived from HTTP cache headers clamped to operator-configured bounds.
type CIMDCache struct {
	mu         sync.RWMutex
	entries    map[string]*CIMDCacheEntry
	minTTL     time.Duration
	maxTTL     time.Duration
	maxEntries int
}

// NewCIMDCache creates a new CIMD cache with the given TTL bounds and entry cap.
// Returns an error if minTTL exceeds maxTTL or maxEntries is not positive.
func NewCIMDCache(minTTL, maxTTL time.Duration, maxEntries int) (*CIMDCache, error) {
	if minTTL > maxTTL {
		return nil, fmt.Errorf("cimd cache: minTTL (%v) must not exceed maxTTL (%v)", minTTL, maxTTL)
	}
	if maxEntries <= 0 {
		return nil, fmt.Errorf("cimd cache: maxEntries must be positive, got %d", maxEntries)
	}
	return &CIMDCache{
		entries:    make(map[string]*CIMDCacheEntry),
		minTTL:     minTTL,
		maxTTL:     maxTTL,
		maxEntries: maxEntries,
	}, nil
}

// Get returns the cached entry for url if it exists and has not expired.
func (c *CIMDCache) Get(url string) *CIMDCacheEntry {
	c.mu.RLock()
	entry, ok := c.entries[url]
	if !ok {
		c.mu.RUnlock()
		return nil
	}
	if !time.Now().After(entry.ExpiresAt) {
		cp := deepCopyEntry(entry)
		c.mu.RUnlock()
		return cp
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	current, ok := c.entries[url]
	if ok && !time.Now().After(current.ExpiresAt) {
		return deepCopyEntry(current)
	}
	if ok {
		delete(c.entries, url)
	}
	return nil
}

// deepCopyDocument returns a deep copy of doc with independent slice fields.
func deepCopyDocument(doc *ClientIDMetadataDocument) *ClientIDMetadataDocument {
	if doc == nil {
		return nil
	}
	cp := *doc
	cp.RedirectURIs = append([]string(nil), doc.RedirectURIs...)
	cp.GrantTypes = append([]string(nil), doc.GrantTypes...)
	cp.ResponseTypes = append([]string(nil), doc.ResponseTypes...)
	if doc.KeySet != nil {
		cloned, err := doc.KeySet.Clone()
		if err == nil {
			cp.KeySet = cloned
		} else {
			cp.KeySet = nil
		}
	}
	return &cp
}

// deepCopyEntry returns a copy of entry with the Document field deep-copied so
// callers cannot mutate cached slice fields (RedirectURIs, GrantTypes, ResponseTypes).
func deepCopyEntry(entry *CIMDCacheEntry) *CIMDCacheEntry {
	cp := *entry
	cp.Document = deepCopyDocument(entry.Document)
	return &cp
}

// Set stores a document in the cache for the given URL, deriving TTL from headers
// clamped to [minTTL, maxTTL]. If the cache is full, the least-recently-fetched
// entry is evicted to make room.
func (c *CIMDCache) Set(url string, doc *ClientIDMetadataDocument, headers CacheHeaders, fetchedAt time.Time) {
	ttl := c.deriveTTL(headers)
	entry := &CIMDCacheEntry{
		URL:       url,
		Document:  deepCopyDocument(doc),
		FetchedAt: fetchedAt,
		ExpiresAt: fetchedAt.Add(ttl),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[url]; !exists && len(c.entries) >= c.maxEntries {
		var oldestKey string
		var oldestTime time.Time
		for k, e := range c.entries {
			if oldestKey == "" || e.FetchedAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.FetchedAt
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[url] = entry
}

// deriveTTL extracts TTL from Cache-Control max-age or Expires headers,
// clamped to [minTTL, maxTTL]. max-age takes precedence over Expires.
func (c *CIMDCache) deriveTTL(headers CacheHeaders) time.Duration {
	ttl := c.minTTL

	if headers.CacheControl != "" {
		if maxAge := extractMaxAge(headers.CacheControl); maxAge > 0 {
			ttl = time.Duration(maxAge) * time.Second
		}
	} else if headers.Expires != "" {
		if t, err := parseHTTPTime(headers.Expires); err == nil && t.After(time.Now()) {
			ttl = time.Until(t)
		}
	}

	if ttl < c.minTTL {
		ttl = c.minTTL
	}
	if ttl > c.maxTTL {
		ttl = c.maxTTL
	}
	return ttl
}

// extractMaxAge parses the max-age directive from a Cache-Control header value.
func extractMaxAge(cc string) int64 {
	for _, part := range strings.Split(cc, ",") {
		part = strings.TrimSpace(part)
		if after, ok := strings.CutPrefix(part, "max-age="); ok {
			if v, err := strconv.ParseInt(strings.TrimSpace(after), 10, 64); err == nil && v >= 0 {
				return v
			}
		}
	}
	return 0
}
