package helpers

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/require"
)

func TestGenerateClientAssertionKey(t *testing.T) {
	for _, algorithm := range []jwa.SignatureAlgorithm{jwa.RS256(), jwa.ES256()} {
		t.Run(algorithm.String(), func(t *testing.T) {
			key, err := GenerateClientAssertionKey(algorithm, "client-key")
			require.NoError(t, err)
			require.Equal(t, "client-key", key.KID)
			require.Equal(t, algorithm, key.Algorithm)

			jwks, err := key.JWKSJSON()
			require.NoError(t, err)
			var document map[string][]map[string]any
			require.NoError(t, json.Unmarshal(jwks, &document))
			require.Len(t, document["keys"], 1)
			require.Equal(t, "client-key", document["keys"][0]["kid"])
			for _, name := range []string{"d", "p", "q", "dp", "dq", "qi", "k"} {
				require.NotContains(t, document["keys"][0], name)
			}
		})
	}
}

func TestSignClientAssertion(t *testing.T) {
	key, err := GenerateClientAssertionKey(jwa.RS256(), "client-key")
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	assertion, gotType, err := SignClientAssertion(
		"https://client.example/metadata.json",
		"https://broker.example/oauth2/token",
		key,
		ClientAssertionOptions{Now: now, JTI: "assertion-1"},
	)
	require.NoError(t, err)
	require.Equal(t, ClientAssertionTypeJWTBearer, gotType)
	require.NotEmpty(t, gotType)

	token, err := jwt.ParseString(assertion, jwt.WithKey(jwa.RS256(), key.PublicKey))
	require.NoError(t, err)
	issuer, _ := token.Issuer()
	subject, _ := token.Subject()
	audience, _ := token.Audience()
	jti, _ := token.JwtID()
	expiration, _ := token.Expiration()
	require.Equal(t, "https://client.example/metadata.json", issuer)
	require.Equal(t, issuer, subject)
	require.Equal(t, []string{"https://broker.example/oauth2/token"}, audience)
	require.Equal(t, "assertion-1", jti)
	require.Equal(t, now.Add(5*time.Minute), expiration)

	parts := splitJWT(assertion)
	require.Len(t, parts, 3)
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var header map[string]any
	require.NoError(t, json.Unmarshal(headerBytes, &header))
	require.Equal(t, "client-key", header["kid"])
}

func TestSignClientAssertionAllowsClaimAndHeaderDefects(t *testing.T) {
	key, err := GenerateClientAssertionKey(jwa.RS256(), "client-key")
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	assertion, assertionType, err := SignClientAssertion(
		"https://client.example/metadata.json",
		"https://broker.example/oauth2/token",
		key,
		ClientAssertionOptions{
			Now:           now,
			Algorithm:     jwa.PS256(),
			Issuer:        "https://wrong-issuer.example",
			Subject:       "wrong-subject",
			Audience:      []string{"https://wrong-audience.example"},
			Lifetime:      10 * time.Minute,
			OmitKID:       true,
			OmitJTI:       true,
			OmitIssuedAt:  true,
			JKU:           "https://attacker.example/keys.json",
			IncludeJWK:    true,
			AssertionType: "urn:wrong-assertion-type",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "urn:wrong-assertion-type", assertionType)
	require.NotEmpty(t, assertion)
	require.Len(t, splitJWT(assertion), 3)

	parts := splitJWT(assertion)
	require.Len(t, parts, 3)
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var header map[string]any
	require.NoError(t, json.Unmarshal(headerBytes, &header))
	require.Equal(t, "PS256", header["alg"])
	require.NotContains(t, header, "kid")
	require.Equal(t, "https://attacker.example/keys.json", header["jku"])
	require.Contains(t, header, "jwk")

	token, err := jwt.ParseString(assertion, jwt.WithKey(jwa.PS256(), key.PublicKey))
	require.NoError(t, err)
	issuer, _ := token.Issuer()
	require.Equal(t, "https://wrong-issuer.example", issuer)
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	require.NotContains(t, payload, "iat")
	require.NotContains(t, payload, "jti")
}

func splitJWT(assertion string) []string {
	return strings.Split(assertion, ".")
}

func TestSignClientAssertionCanCreateEmptyJTI(t *testing.T) {
	key, err := GenerateClientAssertionKey(jwa.RS256(), "client-key")
	require.NoError(t, err)
	assertion, _, err := SignClientAssertion(
		"https://client.example/metadata.json",
		"https://broker.example/oauth2/token",
		key,
		ClientAssertionOptions{EmptyJTI: true},
	)
	require.NoError(t, err)
	parts := splitJWT(assertion)
	require.Len(t, parts, 3)
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	require.Contains(t, payload, "jti")
	require.Empty(t, payload["jti"])
}
