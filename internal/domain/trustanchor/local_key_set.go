package trustanchor

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
)

// MaxKeySetBytes is the size bound for any key-set document, remote or local.
const MaxKeySetBytes = 1 << 20

// LocalKeySet is a validated JWK Set from an inline or file key source.
type LocalKeySet struct {
	Set jwk.Set
	// KeyIDs lists all validated identifiers in document order. It is used only for logs.
	KeyIDs []string
}

// ParseLocalKeySet validates doc against the Local Key Set invariants. Errors carry a fixed
// category and never include document bytes or jwx error text.
func ParseLocalKeySet(doc []byte) (LocalKeySet, error) {
	if len(doc) > MaxKeySetBytes {
		return LocalKeySet{}, newError(CategoryTooLarge)
	}
	if !hasKeysArray(doc) {
		return LocalKeySet{}, newError(CategoryNotAJWKSet)
	}
	set, err := jwk.Parse(doc, jwk.WithStrictKeySetParsing(true))
	if err != nil {
		return LocalKeySet{}, newError(CategoryNotAJWKSet)
	}
	if set.Len() == 0 {
		return LocalKeySet{}, newError(CategoryEmptyKeySet)
	}

	keys := make([]jwk.Key, set.Len())
	for i := range keys {
		keys[i], _ = set.Key(i)
	}
	for _, key := range keys {
		if key.KeyType() == jwa.OctetSeq() {
			return LocalKeySet{}, newError(CategorySymmetricKey)
		}
	}
	for _, key := range keys {
		private, err := jwk.IsPrivateKey(key)
		if err != nil {
			return LocalKeySet{}, newError(CategoryNotAJWKSet)
		}
		if private {
			return LocalKeySet{}, newError(CategoryPrivateKey)
		}
	}
	for i, key := range keys {
		if alg, ok := key.Algorithm(); ok && !IsApprovedAlgorithm(alg.String()) {
			if kid, _ := key.KeyID(); kid != "" {
				return LocalKeySet{}, newError(CategoryUnapprovedAlgorithm, DetailKID, kid)
			}
			return LocalKeySet{}, newError(CategoryUnapprovedAlgorithm, DetailIndex, strconv.Itoa(i))
		}
	}
	var keyIDs []string
	for i, key := range keys {
		kid, _ := key.KeyID()
		if kid == "" {
			return LocalKeySet{}, newError(CategoryMissingKID, DetailIndex, strconv.Itoa(i))
		}
		if slices.Contains(keyIDs, kid) {
			return LocalKeySet{}, newError(CategoryDuplicateKID, DetailKID, kid)
		}
		keyIDs = append(keyIDs, kid)
	}
	return LocalKeySet{Set: set, KeyIDs: keyIDs}, nil
}

// hasKeysArray reports whether doc is a JSON object whose "keys" member is an array. jwx also
// accepts a bare JWK as a one-key set, which RFC 7517 §5 does not.
func hasKeysArray(doc []byte) bool {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(doc, &members); err != nil || members == nil {
		return false
	}
	keys, ok := members["keys"]
	return ok && bytes.HasPrefix(bytes.TrimSpace(keys), []byte("["))
}

// DiffKeyIDs returns the key identifiers added to and removed from previous by next.
func DiffKeyIDs(previous, next LocalKeySet) (added, removed []string) {
	for _, kid := range next.KeyIDs {
		if !slices.Contains(previous.KeyIDs, kid) {
			added = append(added, kid)
		}
	}
	for _, kid := range previous.KeyIDs {
		if !slices.Contains(next.KeyIDs, kid) {
			removed = append(removed, kid)
		}
	}
	return added, removed
}
