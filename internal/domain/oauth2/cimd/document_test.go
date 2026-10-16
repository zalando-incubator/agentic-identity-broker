package cimd

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func publicTestJWKS(t *testing.T) json.RawMessage {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	key, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	set := jwk.NewSet()
	require.NoError(t, set.AddKey(key))
	encoded, err := json.Marshal(set)
	require.NoError(t, err)
	return encoded
}

func validDoc(overrides map[string]any) []byte {
	doc := map[string]any{
		"client_id":     "https://agent.example.com/client",
		"client_name":   "Test Agent",
		"redirect_uris": []string{"https://agent.example.com/callback"},
	}
	for k, v := range overrides {
		if v == nil {
			delete(doc, k)
		} else {
			doc[k] = v
		}
	}
	data, _ := json.Marshal(doc)
	return data
}

const fetchURL = "https://agent.example.com/client"

func TestParseDocument(t *testing.T) {
	t.Run("accepts valid document", func(t *testing.T) {
		doc, err := ParseDocument(validDoc(nil), fetchURL, nil)
		require.NoError(t, err)
		assert.Equal(t, fetchURL, doc.ClientID)
		assert.Equal(t, "Test Agent", doc.ClientName)
		assert.Equal(t, []string{"https://agent.example.com/callback"}, doc.RedirectURIs)
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		_, err := ParseDocument([]byte("not json"), fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed")
	})

	t.Run("rejects client_id mismatch", func(t *testing.T) {
		data := validDoc(map[string]any{"client_id": "https://other.example.com/client"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client_id mismatch")
	})

	t.Run("rejects empty redirect_uris", func(t *testing.T) {
		data := validDoc(map[string]any{"redirect_uris": []string{}})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "redirect_uris")
	})

	t.Run("rejects missing redirect_uris", func(t *testing.T) {
		data := validDoc(map[string]any{"redirect_uris": nil})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "redirect_uris")
	})

	t.Run("rejects client_secret_post auth method", func(t *testing.T) {
		data := validDoc(map[string]any{"token_endpoint_auth_method": "client_secret_post"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client_secret_post")
	})

	t.Run("rejects client_secret_basic auth method", func(t *testing.T) {
		data := validDoc(map[string]any{"token_endpoint_auth_method": "client_secret_basic"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
	})

	t.Run("rejects client_secret_jwt auth method", func(t *testing.T) {
		data := validDoc(map[string]any{"token_endpoint_auth_method": "client_secret_jwt"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
	})

	t.Run("accepts none auth method", func(t *testing.T) {
		data := validDoc(map[string]any{"token_endpoint_auth_method": "none"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
	})

	t.Run("accepts private_key_jwt with an HTTPS jwks_uri", func(t *testing.T) {
		data := validDoc(map[string]any{
			"token_endpoint_auth_method": "private_key_jwt",
			"jwks_uri":                   "https://agent.example.com/keys/jwks.json",
		})
		doc, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
		assert.Equal(t, "RS256", doc.SigningAlg)
		assert.Equal(t, "https://agent.example.com/keys/jwks.json", doc.JWKSURI)
	})

	t.Run("accepts private_key_jwt with an inline public jwks", func(t *testing.T) {
		data := validDoc(map[string]any{
			"token_endpoint_auth_method": "private_key_jwt",
			"jwks":                       publicTestJWKS(t),
		})
		doc, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
		require.NotNil(t, doc.KeySet)
		assert.Equal(t, 1, doc.KeySet.Len())
	})

	t.Run("requires exactly one key source", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			overrides map[string]any
		}{
			{name: "missing", overrides: map[string]any{"token_endpoint_auth_method": "private_key_jwt"}},
			{name: "both", overrides: map[string]any{
				"token_endpoint_auth_method": "private_key_jwt",
				"jwks_uri":                   "https://agent.example.com/keys/jwks.json",
				"jwks":                       publicTestJWKS(t),
			}},
		} {
			t.Run(test.name, func(t *testing.T) {
				_, err := ParseDocument(validDoc(test.overrides), fetchURL, nil)
				require.Error(t, err)
			})
		}
	})

	t.Run("validates jwks_uri using CIMD URL rules", func(t *testing.T) {
		for _, rawURL := range []string{
			"http://agent.example.com/keys.json",
			"https://agent.example.com:8443/keys.json",
			"https://user:pass@agent.example.com/keys.json",
			"https://agent.example.com/keys.json#fragment",
			"https://agent.example.com/a/../keys.json",
		} {
			t.Run(rawURL, func(t *testing.T) {
				data := validDoc(map[string]any{
					"token_endpoint_auth_method": "private_key_jwt",
					"jwks_uri":                   rawURL,
				})
				_, err := ParseDocument(data, fetchURL, nil)
				require.Error(t, err)
			})
		}
	})

	t.Run("accepts only RS256 and ES256 signing algorithms", func(t *testing.T) {
		for _, algorithm := range []string{"none", "HS256", "PS256"} {
			data := validDoc(map[string]any{
				"token_endpoint_auth_method":      "private_key_jwt",
				"token_endpoint_auth_signing_alg": algorithm,
				"jwks_uri":                        "https://agent.example.com/keys/jwks.json",
			})
			_, err := ParseDocument(data, fetchURL, nil)
			require.Error(t, err, "algorithm %s must be rejected", algorithm)
		}
		for _, algorithm := range []string{"RS256", "ES256"} {
			data := validDoc(map[string]any{
				"token_endpoint_auth_method":      "private_key_jwt",
				"token_endpoint_auth_signing_alg": algorithm,
				"jwks_uri":                        "https://agent.example.com/keys/jwks.json",
			})
			doc, err := ParseDocument(data, fetchURL, nil)
			require.NoError(t, err)
			assert.Equal(t, algorithm, doc.SigningAlg)
		}
	})

	t.Run("rejects inline jwks containing private key material", func(t *testing.T) {
		for _, member := range []string{"d", "p", "q", "dp", "dq", "qi", "k"} {
			t.Run(member, func(t *testing.T) {
				var set map[string][]map[string]any
				require.NoError(t, json.Unmarshal(publicTestJWKS(t), &set))
				set["keys"][0][member] = "AQAB"
				data := validDoc(map[string]any{
					"token_endpoint_auth_method": "private_key_jwt",
					"jwks":                       set,
				})
				_, err := ParseDocument(data, fetchURL, nil)
				require.Error(t, err, "private JWK member %s must be rejected", member)
			})
		}
	})

	t.Run("ignores the plural method field for selection", func(t *testing.T) {
		data := validDoc(map[string]any{
			"token_endpoint_auth_method":            "private_key_jwt",
			"token_endpoint_auth_methods_supported": []string{"none", "private_key_jwt"},
			"jwks_uri":                              "https://agent.example.com/keys/jwks.json",
		})
		doc, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
		assert.Equal(t, "private_key_jwt", doc.AuthMethod)
	})

	t.Run("ignores key sources for none clients", func(t *testing.T) {
		data := validDoc(map[string]any{
			"token_endpoint_auth_method": "none",
			"jwks_uri":                   "not a valid URL",
			"jwks":                       []byte("not a key set"),
		})
		doc, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
		assert.Equal(t, "none", doc.AuthMethod)
		assert.Nil(t, doc.KeySet)
	})

	t.Run("rejects unknown auth method", func(t *testing.T) {
		data := validDoc(map[string]any{"token_endpoint_auth_method": "tls_client_auth"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tls_client_auth")
	})

	t.Run("rejects document containing client_secret", func(t *testing.T) {
		data := validDoc(map[string]any{"client_secret": "s3cret"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client_secret")
	})

	t.Run("rejects document containing client_secret_expires_at", func(t *testing.T) {
		data := validDoc(map[string]any{"client_secret_expires_at": 1234567890})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client_secret_expires_at")
	})

	t.Run("rejects blocked client_name keyword", func(t *testing.T) {
		data := validDoc(map[string]any{"client_name": "Admin"})
		_, err := ParseDocument(data, fetchURL, []string{"Admin"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked term")
	})

	t.Run("keyword check is case-insensitive", func(t *testing.T) {
		data := validDoc(map[string]any{"client_name": "ADMIN"})
		_, err := ParseDocument(data, fetchURL, []string{"admin"})
		require.Error(t, err)
	})

	t.Run("partial name not blocked by exact match", func(t *testing.T) {
		data := validDoc(map[string]any{"client_name": "Admin Panel"})
		_, err := ParseDocument(data, fetchURL, []string{"admin"})
		require.NoError(t, err, "partial substring must not trigger exact match blocklist")
	})

	t.Run("allows client_name not in blocklist", func(t *testing.T) {
		data := validDoc(map[string]any{"client_name": "Helpful Assistant"})
		_, err := ParseDocument(data, fetchURL, []string{"admin"})
		require.NoError(t, err)
	})

	t.Run("rejects cross-origin redirect URI", func(t *testing.T) {
		data := validDoc(map[string]any{
			"redirect_uris": []string{"https://other.example.com/callback"},
		})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "same-origin")
	})

	t.Run("allows localhost redirect URI (exception)", func(t *testing.T) {
		data := validDoc(map[string]any{
			"redirect_uris": []string{
				"https://agent.example.com/callback",
				"http://localhost/callback",
			},
		})
		_, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
	})

	t.Run("allows 127.0.0.1 redirect URI (exception)", func(t *testing.T) {
		data := validDoc(map[string]any{
			"redirect_uris": []string{"http://127.0.0.1/callback"},
		})
		_, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
	})

	t.Run("accepts logo_uri on same host", func(t *testing.T) {
		data := validDoc(map[string]any{"logo_uri": "https://agent.example.com/logo.png"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
	})

	t.Run("accepts logo_uri on different host", func(t *testing.T) {
		data := validDoc(map[string]any{"logo_uri": "https://cdn.example.com/logo.png"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
	})

	t.Run("accepts logo_uri on same host with different port", func(t *testing.T) {
		data := validDoc(map[string]any{"logo_uri": "https://agent.example.com:8443/logo.png"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.NoError(t, err)
	})

	t.Run("rejects http logo_uri", func(t *testing.T) {
		data := validDoc(map[string]any{"logo_uri": "http://agent.example.com/logo.png"})
		_, err := ParseDocument(data, fetchURL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTPS")
	})
}
