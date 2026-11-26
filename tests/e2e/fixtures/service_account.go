package fixtures

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jws"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

// Values of the Company Platform journeys in
// specs/20261125-152113-service-accounts-stored-jwt/spec.md. All tokens are synthetic and signed
// with a P-256 key generated per call.
const (
	CompanyPlatformCanonicalID  = "company-platform"
	ServiceAccountTokenIssuer   = "https://idp.example.com"
	ServiceAccountTokenSubject  = "triage-bot"
	ServiceAccountTokenAudience = "https://platform.example.com"
	ServiceAccountT1KeyID       = "2026-11"
	ServiceAccountT2KeyID       = "2026-12"
	ServiceAccountJWTMaxBytes   = 16384
)

// CompanyPlatformService returns the existing third-party service of the journeys with a fresh ID.
func CompanyPlatformService() *model.ThirdpartyOAuth2ProviderEntity {
	return NamedPlatformService(CompanyPlatformCanonicalID)
}

// NamedPlatformService returns a service with a fresh ID and the given canonical ID.
func NamedPlatformService(canonicalID string) *model.ThirdpartyOAuth2ProviderEntity {
	service := ServiceWithID(id.NewServiceID().String())
	service.DisplayName = "Company Platform " + canonicalID
	service.CanonicalID = &canonicalID
	service.ProtectedResources = []string{"https://" + canonicalID + ".example.com/api"}
	return service
}

// ServiceAccountTokenT1 returns token T1 of the spec: ES256, kid 2026-11, expiring in 30 days.
func ServiceAccountTokenT1() string {
	return SignedServiceAccountToken(ServiceAccountT1KeyID, journeyClaims(time.Now().Add(30*24*time.Hour)))
}

// ServiceAccountTokenT2 returns the replacement token T2: kid 2026-12, expiring in 60 days.
func ServiceAccountTokenT2() string {
	return SignedServiceAccountToken(ServiceAccountT2KeyID, journeyClaims(time.Now().Add(60*24*time.Hour)))
}

// ServiceAccountTokenWithoutExp returns a signed token that states no exp.
func ServiceAccountTokenWithoutExp() string {
	claims := journeyClaims(time.Now())
	delete(claims, "exp")
	return SignedServiceAccountToken(ServiceAccountT1KeyID, claims)
}

// ServiceAccountTokenExpiringIn returns a signed token whose exp lies the given duration ahead,
// rounded up to whole seconds. A negative duration yields an expired token.
func ServiceAccountTokenExpiringIn(ahead time.Duration) string {
	return SignedServiceAccountToken(ServiceAccountT1KeyID, journeyClaims(time.Now().Add(ahead).Add(time.Second-1)))
}

// ServiceAccountTokenExpiringAt returns a signed token with the given exp in whole seconds.
func ServiceAccountTokenExpiringAt(exp int64) string {
	claims := journeyClaims(time.Now())
	claims["exp"] = exp
	return SignedServiceAccountToken(ServiceAccountT1KeyID, claims)
}

// SignedServiceAccountToken signs claims with ES256 and a fresh key; kid is omitted when empty.
func SignedServiceAccountToken(kid string, claims map[string]any) string {
	payload, err := json.Marshal(claims)
	if err != nil {
		panic(fmt.Sprintf("marshal service account claims: %v", err))
	}
	return signServiceAccountPayload(kid, payload)
}

func signServiceAccountPayload(kid string, payload []byte) string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("generate service account signing key: %v", err))
	}
	headers := jws.NewHeaders()
	if err := headers.Set("typ", "JWT"); err != nil {
		panic(err)
	}
	if kid != "" {
		if err := headers.Set("kid", kid); err != nil {
			panic(err)
		}
	}
	signed, err := jws.Sign(payload, jws.WithKey(jwa.ES256(), key, jws.WithProtectedHeaders(headers)))
	if err != nil {
		panic(fmt.Sprintf("sign service account token: %v", err))
	}
	return string(signed)
}

func journeyClaims(exp time.Time) map[string]any {
	return map[string]any{
		"iss":   ServiceAccountTokenIssuer,
		"sub":   ServiceAccountTokenSubject,
		"aud":   ServiceAccountTokenAudience,
		"scope": "read",
		"iat":   time.Now().Unix(),
		"exp":   exp.Unix(),
	}
}

func encodeSegment(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

// UnsignedServiceAccountTokens returns tokens whose header states alg none in several letter
// cases, or no alg, keyed by variant. All carry the claims of T1.
func UnsignedServiceAccountTokens() map[string]string {
	payload, err := json.Marshal(journeyClaims(time.Now().Add(time.Hour)))
	if err != nil {
		panic(err)
	}
	claims := base64.RawURLEncoding.EncodeToString(payload)
	signature := strings.Split(ServiceAccountTokenT1(), ".")[2]
	return map[string]string{
		"alg none":    encodeSegment(`{"alg":"none"}`) + "." + claims + ".",
		"alg NONE":    encodeSegment(`{"alg":"NONE"}`) + "." + claims + ".",
		"alg None":    encodeSegment(`{"alg":"None"}`) + "." + claims + ".",
		"missing alg": encodeSegment(`{"typ":"JWT","kid":"2026-11"}`) + "." + claims + "." + signature,
	}
}

// MalformedServiceAccountTokens returns tokens that are not a compact JWS with JWT claims, keyed
// by variant.
func MalformedServiceAccountTokens() map[string]string {
	valid := ServiceAccountTokenT1()
	segments := strings.Split(valid, ".")
	return map[string]string{
		"two segments":              segments[0] + "." + segments[1],
		"five segments":             valid + "." + segments[1] + "." + segments[2],
		"undecodable segment":       segments[0] + ".!!not-base64!!." + segments[2],
		"leading space":             " " + valid,
		"payload not a JSON object": signServiceAccountPayload(ServiceAccountT1KeyID, []byte(`["read","write"]`)),
		"header not a JSON object":  encodeSegment(`["ES256"]`) + "." + segments[1] + "." + segments[2],
		"boolean exp":               signServiceAccountPayload(ServiceAccountT1KeyID, []byte(`{"iss":"https://idp.example.com","exp":true}`)),
	}
}

// OversizedServiceAccountToken returns a signed token of exactly ServiceAccountJWTMaxBytes+1 bytes.
func OversizedServiceAccountToken() string {
	return serviceAccountTokenOfLength(ServiceAccountJWTMaxBytes + 1)
}

func serviceAccountTokenOfLength(length int) string {
	for kidPadding := 0; kidPadding < 4; kidPadding++ {
		kid := ServiceAccountT1KeyID + strings.Repeat("k", kidPadding)
		claims := journeyClaims(time.Now().Add(time.Hour))
		claims["padding"] = ""
		base := len(SignedServiceAccountToken(kid, claims))
		claims["padding"] = strings.Repeat("p", (length-base)*3/4)
		for range 8 {
			token := SignedServiceAccountToken(kid, claims)
			switch {
			case len(token) == length:
				return token
			case len(token) < length:
				claims["padding"] = claims["padding"].(string) + "p"
			default:
				claims["padding"] = strings.TrimSuffix(claims["padding"].(string), "p")
			}
		}
	}
	panic(fmt.Sprintf("cannot build a service account token of %d bytes", length))
}

// ServiceAccountCreateBody returns a create request with a signed_jwt credential that states
// use access_token.
func ServiceAccountCreateBody(canonicalID string, scopes []string, jwt string) map[string]any {
	return map[string]any{
		"canonical_id": canonicalID,
		"kind":         "jwt_bearer",
		"scopes":       scopes,
		"credential":   map[string]any{"format": "signed_jwt", "use": "access_token", "jwt": jwt},
	}
}

// ServiceAccountUpdateBody returns an update request; the credential is omitted when jwt is empty.
func ServiceAccountUpdateBody(scopes []string, jwt string) map[string]any {
	body := map[string]any{"scopes": scopes}
	if jwt != "" {
		body["credential"] = map[string]any{"format": "signed_jwt", "jwt": jwt}
	}
	return body
}
