package cimd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
)

var ErrKeySetUnavailable = errors.New("client key set unavailable")

const maxClientKeySetCacheEntries = 1000

type clientKeySetCacheKey struct {
	clientID string
	source   string
}

type clientKeySetCacheEntry struct {
	keys          jwk.Set
	fetchedAt     time.Time
	lastRefetchAt time.Time
}

// ClientKeySetService retrieves, filters, and caches public keys published by a CIMD client.
type ClientKeySetService struct {
	mu      sync.Mutex
	fetcher ports.CIMDFetcher
	config  ports.CIMDClientAuthenticationConfig
	clock   func() time.Time
	logger  *slog.Logger
	cache   map[clientKeySetCacheKey]*clientKeySetCacheEntry
	refresh singleflight.Group
}

// NewClientKeySetService creates a key-set service with an injectable clock.
func NewClientKeySetService(fetcher ports.CIMDFetcher, config ports.CIMDClientAuthenticationConfig, clock func() time.Time, logger *slog.Logger) *ClientKeySetService {
	if clock == nil {
		clock = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ClientKeySetService{
		fetcher: fetcher,
		config:  config,
		clock:   clock,
		logger:  logger,
		cache:   make(map[clientKeySetCacheKey]*clientKeySetCacheEntry),
	}
}

// KeysFor returns eligible public verification keys for a client's current document.
func (s *ClientKeySetService) KeysFor(ctx context.Context, document *ports.CIMDMetadataDTO) (jwk.Set, error) {
	if document == nil || document.ClientID == "" {
		return nil, fmt.Errorf("%w: client metadata is required", ErrKeySetUnavailable)
	}
	algorithm := document.SigningAlg
	if algorithm == "" {
		algorithm = "RS256"
	}
	if document.KeySet != nil {
		return eligibleClientKeys(document.KeySet, algorithm)
	}
	if document.JWKSURI == "" || s.fetcher == nil {
		return nil, fmt.Errorf("%w: no key source is available", ErrKeySetUnavailable)
	}

	key := clientKeySetCacheKey{clientID: document.ClientID, source: document.JWKSURI}
	now := s.clock()
	s.mu.Lock()
	entry := s.cache[key]
	if entry != nil && now.Sub(entry.fetchedAt) < s.config.KeySetCacheTTL {
		keys, err := eligibleClientKeys(entry.keys, algorithm)
		s.mu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("%w: clone cached key set: %v", ErrKeySetUnavailable, err)
		}
		return keys, nil
	}
	s.mu.Unlock()

	result, err := s.fetcher.Fetch(ctx, document.JWKSURI)
	if err != nil {
		s.auditRefresh(document, 0, "unavailable")
		return nil, fmt.Errorf("%w: fetch key set: %v", ErrKeySetUnavailable, err)
	}
	if result == nil || len(result.Body) == 0 || len(result.Body) > s.config.KeySetMaxBytes {
		s.auditRefresh(document, 0, "unavailable")
		return nil, fmt.Errorf("%w: key set response is empty or exceeds %d bytes", ErrKeySetUnavailable, s.config.KeySetMaxBytes)
	}
	parsed, err := jwk.Parse(result.Body)
	if err != nil {
		s.auditRefresh(document, 0, "unavailable")
		return nil, fmt.Errorf("%w: parse key set: %v", ErrKeySetUnavailable, err)
	}
	eligible, err := eligibleClientKeys(parsed, "")
	if err != nil {
		s.auditRefresh(document, 0, "unavailable")
		return nil, fmt.Errorf("%w: filter key set: %v", ErrKeySetUnavailable, err)
	}

	fetchedAt := s.clock()
	s.storeCacheEntry(key, eligible, fetchedAt, fetchedAt)
	currentKeys, err := eligibleClientKeys(eligible, algorithm)
	if err != nil {
		return nil, fmt.Errorf("%w: filter cached key set: %v", ErrKeySetUnavailable, err)
	}
	s.auditRefresh(document, currentKeys.Len(), "success")
	return currentKeys, nil
}

// RefreshForKID refreshes a remote key set for an unknown key when its minimum
// refetch interval has elapsed. Concurrent callers share one refresh.
func (s *ClientKeySetService) RefreshForKID(ctx context.Context, document *ports.CIMDMetadataDTO, kid string) (jwk.Set, error) {
	keys, err := s.KeysFor(ctx, document)
	if err != nil {
		return nil, err
	}
	if _, found := keys.LookupKeyID(kid); found || document.KeySet != nil || document.JWKSURI == "" {
		return keys, nil
	}

	cacheKey := clientKeySetCacheKey{clientID: document.ClientID, source: document.JWKSURI}
	flightKey := cacheKey.clientID + "\x00" + cacheKey.source
	result, err, _ := s.refresh.Do(flightKey, func() (any, error) {
		now := s.clock()
		s.mu.Lock()
		entry := s.cache[cacheKey]
		if entry == nil {
			s.mu.Unlock()
			return keys, nil
		}
		if now.Sub(entry.lastRefetchAt) < s.config.KeySetMinRefetchInterval {
			cached, cloneErr := cloneJWKSet(entry.keys)
			s.mu.Unlock()
			if cloneErr != nil {
				return nil, fmt.Errorf("%w: clone cached key set: %v", ErrKeySetUnavailable, cloneErr)
			}
			return cached, nil
		}
		entry.lastRefetchAt = now
		s.mu.Unlock()

		algorithm := document.SigningAlg
		if algorithm == "" {
			algorithm = "RS256"
		}
		fetched, fetchErr := s.fetcher.Fetch(ctx, document.JWKSURI)
		if fetchErr != nil {
			s.auditRefresh(document, 0, "unavailable")
			return nil, fmt.Errorf("%w: fetch key set: %v", ErrKeySetUnavailable, fetchErr)
		}
		if fetched == nil || len(fetched.Body) == 0 || len(fetched.Body) > s.config.KeySetMaxBytes {
			s.auditRefresh(document, 0, "unavailable")
			return nil, fmt.Errorf("%w: key set response is empty or exceeds %d bytes", ErrKeySetUnavailable, s.config.KeySetMaxBytes)
		}
		parsed, parseErr := jwk.Parse(fetched.Body)
		if parseErr != nil {
			s.auditRefresh(document, 0, "unavailable")
			return nil, fmt.Errorf("%w: parse key set: %v", ErrKeySetUnavailable, parseErr)
		}
		eligible, filterErr := eligibleClientKeys(parsed, "")
		if filterErr != nil {
			s.auditRefresh(document, 0, "unavailable")
			return nil, fmt.Errorf("%w: filter key set: %v", ErrKeySetUnavailable, filterErr)
		}

		fetchedAt := s.clock()
		s.storeCacheEntry(cacheKey, eligible, fetchedAt, now)
		currentKeys, filterErr := eligibleClientKeys(eligible, algorithm)
		if filterErr != nil {
			return nil, fmt.Errorf("%w: filter cached key set: %v", ErrKeySetUnavailable, filterErr)
		}
		s.auditRefresh(document, currentKeys.Len(), "success")
		return currentKeys, nil
	})
	if err != nil {
		return nil, err
	}
	refreshed, ok := result.(jwk.Set)
	if !ok {
		return nil, fmt.Errorf("%w: invalid refresh result", ErrKeySetUnavailable)
	}
	return refreshed, nil
}

func (s *ClientKeySetService) storeCacheEntry(key clientKeySetCacheKey, keys jwk.Set, fetchedAt, lastRefetchAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous := s.cache[key]; previous != nil {
		lastRefetchAt = previous.lastRefetchAt
	} else if len(s.cache) >= maxClientKeySetCacheEntries {
		var oldestKey clientKeySetCacheKey
		var oldestTime time.Time
		for candidate, entry := range s.cache {
			if oldestKey == (clientKeySetCacheKey{}) || entry.fetchedAt.Before(oldestTime) {
				oldestKey = candidate
				oldestTime = entry.fetchedAt
			}
		}
		delete(s.cache, oldestKey)
	}
	s.cache[key] = &clientKeySetCacheEntry{keys: keys, fetchedAt: fetchedAt, lastRefetchAt: lastRefetchAt}
}

func eligibleClientKeys(source jwk.Set, signingAlgorithm string) (jwk.Set, error) {
	filtered := jwk.NewSet()
	if source == nil {
		return filtered, nil
	}
	for index := 0; index < source.Len(); index++ {
		key, ok := source.Key(index)
		if !ok || !clientKeyEligible(key, signingAlgorithm) {
			continue
		}
		publicKey, err := key.PublicKey()
		if err != nil {
			continue
		}
		if err := filtered.AddKey(publicKey); err != nil {
			return nil, err
		}
	}
	return filtered, nil
}

// ClientKeyEligible reports whether a public key meets the verifier's supported key profile.
func ClientKeyEligible(key jwk.Key, signingAlgorithm string) bool {
	return clientKeyEligible(key, signingAlgorithm)
}

func clientKeyEligible(key jwk.Key, signingAlgorithm string) bool {
	if key == nil || hasPrivateKeyMaterial(key) {
		return false
	}
	if use, ok := key.KeyUsage(); ok && use != "sig" {
		return false
	}
	if operations, ok := key.KeyOps(); ok {
		canVerify := false
		for _, operation := range operations {
			if operation == jwk.KeyOpVerify {
				canVerify = true
				break
			}
		}
		if !canVerify {
			return false
		}
	}

	switch {
	case (signingAlgorithm == "RS256" || signingAlgorithm == "") && key.KeyType() == jwa.RSA():
		rsaKey, ok := key.(jwk.RSAPublicKey)
		if !ok {
			return false
		}
		modulus, ok := rsaKey.N()
		return ok && new(big.Int).SetBytes(modulus).BitLen() >= 2048
	case (signingAlgorithm == "ES256" || signingAlgorithm == "") && key.KeyType() == jwa.EC():
		ecKey, ok := key.(jwk.ECDSAPublicKey)
		if !ok {
			return false
		}
		curve, ok := ecKey.Crv()
		return ok && curve == jwa.P256()
	default:
		return false
	}
}

func hasPrivateKeyMaterial(key jwk.Key) bool {
	privateMembers := map[string]struct{}{
		"d": {}, "p": {}, "q": {}, "dp": {}, "dq": {}, "qi": {}, "k": {},
	}
	for _, member := range key.Keys() {
		if _, found := privateMembers[member]; found {
			return true
		}
	}
	return false
}

func cloneJWKSet(set jwk.Set) (jwk.Set, error) {
	if set == nil {
		return nil, nil
	}
	return set.Clone()
}

func (s *ClientKeySetService) auditRefresh(document *ports.CIMDMetadataDTO, keyCount int, outcome string) {
	sourceHost := "inline"
	if document != nil && document.JWKSURI != "" {
		if parsed, err := url.Parse(document.JWKSURI); err == nil {
			sourceHost = parsed.Hostname()
		}
	}
	clientID := ""
	if document != nil {
		clientID = document.ClientID
	}
	s.logger.Info("client_key_set_refreshed", "client_id", clientID, "source_host", sourceHost, "outcome", outcome, "key_count", keyCount)
}
