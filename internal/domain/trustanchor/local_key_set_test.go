package trustanchor

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testKey struct {
	public  map[string]any
	private map[string]any
}

func jwkMap(t *testing.T, raw any, kid, alg string) map[string]any {
	t.Helper()
	key, err := jwk.Import[jwk.Key](raw)
	require.NoError(t, err)
	encoded, err := json.Marshal(key)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(encoded, &out))
	if kid != "" {
		out["kid"] = kid
	}
	if alg != "" {
		out["alg"] = alg
	}
	return out
}

func newECKey(t *testing.T, kid string) testKey {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return testKey{public: jwkMap(t, &private.PublicKey, kid, "ES256"), private: jwkMap(t, private, kid, "ES256")}
}

func newRSAKey(t *testing.T, kid string) testKey {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return testKey{public: jwkMap(t, &private.PublicKey, kid, "RS256"), private: jwkMap(t, private, kid, "RS256")}
}

func newEdKey(t *testing.T, kid string) testKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return testKey{public: jwkMap(t, public, kid, "EdDSA"), private: jwkMap(t, private, kid, "EdDSA")}
}

func keySet(t *testing.T, keys ...map[string]any) []byte {
	t.Helper()
	doc, err := json.Marshal(map[string]any{"keys": keys})
	require.NoError(t, err)
	return doc
}

func withField(key map[string]any, name string, value any) map[string]any {
	out := make(map[string]any, len(key)+1)
	for k, v := range key {
		out[k] = v
	}
	if value == nil {
		delete(out, name)
	} else {
		out[name] = value
	}
	return out
}

func TestParseLocalKeySetAcceptsPublicAsymmetricKeys(t *testing.T) {
	ec, rsaKey, ed := newECKey(t, "ec-1"), newRSAKey(t, "rsa-1"), newEdKey(t, "ed-1")

	set, err := ParseLocalKeySet(keySet(t, ec.public, rsaKey.public, ed.public))
	require.NoError(t, err)
	assert.Equal(t, 3, set.Set.Len())
	assert.Equal(t, []string{"ec-1", "rsa-1", "ed-1"}, set.KeyIDs)

	t.Run("keys with kid but without alg are accepted", func(t *testing.T) {
		withoutAlg := withField(ec.public, "alg", nil)
		set, err := ParseLocalKeySet(keySet(t, withoutAlg, rsaKey.public))
		require.NoError(t, err)
		assert.Equal(t, 2, set.Set.Len())
		assert.Equal(t, []string{"ec-1", "rsa-1"}, set.KeyIDs)
	})

	t.Run("exactly the size bound is accepted", func(t *testing.T) {
		doc := keySet(t, ec.public)
		padded := append(doc[:len(doc)-1:len(doc)-1], []byte(`,"pad":"`+strings.Repeat("a", MaxKeySetBytes-len(doc)-9)+`"}`)...)
		require.Len(t, padded, MaxKeySetBytes)
		_, err := ParseLocalKeySet(padded)
		require.NoError(t, err)
	})
}

func TestParseLocalKeySetInvariants(t *testing.T) {
	ec, other, rsaKey := newECKey(t, "gw-1"), newECKey(t, "gw-2"), newRSAKey(t, "rsa-1")
	oversized := keySet(t, ec.public)
	oversized = append(oversized[:len(oversized)-1:len(oversized)-1],
		[]byte(`,"pad":"`+strings.Repeat("a", MaxKeySetBytes)+`"}`)...)

	tests := []struct {
		name    string
		doc     []byte
		want    Category
		message string
	}{
		{name: "1: larger than 1 MiB", doc: oversized, want: CategoryTooLarge,
			message: "key set exceeds 1048576 bytes"},
		{name: "2: bare JWK without keys", doc: mustJSON(t, ec.public), want: CategoryNotAJWKSet,
			message: "key set is not a valid JWK Set document"},
		{name: "2: keys is not an array", doc: []byte(`{"keys":{"kty":"EC"}}`), want: CategoryNotAJWKSet},
		{name: "2: top level is an array", doc: []byte(`[]`), want: CategoryNotAJWKSet},
		{name: "2: malformed JSON", doc: []byte(`{"keys":["`), want: CategoryNotAJWKSet},
		{name: "2: empty document", doc: nil, want: CategoryNotAJWKSet},
		{name: "3: unknown key type", doc: keySet(t, map[string]any{"kty": "XYZ", "kid": "x"}), want: CategoryNotAJWKSet},
		{name: "3: invalid key members", doc: keySet(t, withField(ec.public, "x", "not-base64!")), want: CategoryNotAJWKSet},
		{name: "4: no keys", doc: []byte(`{"keys":[]}`), want: CategoryEmptyKeySet,
			message: "key set contains no keys"},
		{name: "5: symmetric oct key", doc: keySet(t, ec.public, map[string]any{"kty": "oct", "kid": "shared", "k": "c2VjcmV0LXNlY3JldC1zZWNyZXQtc2VjcmV0"}),
			want: CategorySymmetricKey, message: "key set contains a symmetric key; only public asymmetric keys are allowed"},
		{name: "6: private EC key", doc: keySet(t, ec.private), want: CategoryPrivateKey,
			message: "key set contains private key material; only public keys are allowed"},
		{name: "6: private RSA key", doc: keySet(t, ec.public, rsaKey.private), want: CategoryPrivateKey},
		{name: "6: private Ed25519 key", doc: keySet(t, newEdKey(t, "ed").private), want: CategoryPrivateKey},
		{name: "7: unapproved alg names the kid", doc: keySet(t, withField(ec.public, "alg", "HS256")),
			want: CategoryUnapprovedAlgorithm, message: `key "gw-1" declares an algorithm outside the approved asymmetric set`},
		{name: "7: unapproved alg without kid names the index", doc: keySet(t, ec.public, withField(withField(other.public, "kid", nil), "alg", "none")),
			want: CategoryUnapprovedAlgorithm, message: "key at index 1 declares an algorithm outside the approved asymmetric set"},
		{name: "8: missing kid at index zero", doc: keySet(t, withField(ec.public, "kid", nil)),
			want: CategoryMissingKID, message: "key at index 0 requires a non-empty kid"},
		{name: "8: empty kid at index zero", doc: keySet(t, withField(ec.public, "kid", "")),
			want: CategoryMissingKID, message: "key at index 0 requires a non-empty kid"},
		{name: "8: missing kid at index one", doc: keySet(t, ec.public, withField(other.public, "kid", nil)),
			want: CategoryMissingKID, message: "key at index 1 requires a non-empty kid"},
		{name: "8: duplicate kid", doc: keySet(t, withField(ec.public, "kid", "dup-1"), withField(other.public, "kid", "dup-1")),
			want: CategoryDuplicateKID, message: `key identifier "dup-1" appears more than once`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseLocalKeySet(tc.doc)
			anchorErr := requireCategory(t, err, tc.want)
			if tc.message != "" {
				assert.Equal(t, tc.message, anchorErr.Error())
			}
		})
	}
}

func TestParseLocalKeySetErrorsLeakNoContent(t *testing.T) {
	ec, rsaKey := newECKey(t, "leak-1"), newRSAKey(t, "leak-2")
	const marker = "DOCUMENT-MARKER-91c2"
	docs := [][]byte{
		keySet(t, ec.private),
		keySet(t, rsaKey.private),
		[]byte(`{"keys":["` + marker),
		keySet(t, map[string]any{"kty": marker}),
		keySet(t, withField(ec.public, "x", marker)),
		keySet(t, map[string]any{"kty": "oct", "k": marker}),
	}
	secrets := []string{marker}
	for _, key := range []testKey{ec, rsaKey} {
		for _, member := range []string{"d", "x", "y", "n", "p", "q", "dp", "dq", "qi"} {
			if value, ok := key.private[member].(string); ok {
				secrets = append(secrets, value)
			}
		}
	}

	for i, doc := range docs {
		_, err := ParseLocalKeySet(doc)
		require.Error(t, err, "document %d", i)
		for _, secret := range secrets {
			assert.NotContains(t, err.Error(), secret, "document %d", i)
		}
		assert.NotContains(t, err.Error(), "jwk.", "jwx error text must not leak (document %d)", i)
	}
}

func TestDiffKeyIDs(t *testing.T) {
	a, b, c := newECKey(t, "a"), newECKey(t, "b"), newECKey(t, "c")
	previous, err := ParseLocalKeySet(keySet(t, a.public, b.public))
	require.NoError(t, err)
	next, err := ParseLocalKeySet(keySet(t, b.public, c.public))
	require.NoError(t, err)

	added, removed := DiffKeyIDs(previous, next)
	assert.Equal(t, []string{"c"}, added)
	assert.Equal(t, []string{"a"}, removed)

	added, removed = DiffKeyIDs(next, next)
	assert.Empty(t, added)
	assert.Empty(t, removed)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	doc, err := json.Marshal(value)
	require.NoError(t, err)
	return doc
}
