package model

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var credentialTestNow = time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

func signTestJWT(t *testing.T, kid string, payload string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	headers := jws.NewHeaders()
	require.NoError(t, headers.Set("typ", "JWT"))
	if kid != "" {
		require.NoError(t, headers.Set("kid", kid))
	}
	signed, err := jws.Sign([]byte(payload), jws.WithKey(jwa.ES256(), key, jws.WithProtectedHeaders(headers)))
	require.NoError(t, err)
	return string(signed)
}

func segment(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func t1Payload(exp int64) string {
	encoded, _ := json.Marshal(map[string]any{
		"iss": "https://idp.example.com", "sub": "triage-bot", "aud": "https://platform.example.com",
		"scope": "read", "iat": credentialTestNow.Unix() - 60, "exp": exp,
	})
	return string(encoded)
}

func signedInput(jwt string) ServiceAccountCredentialInput {
	return ServiceAccountCredentialInput{Format: CredentialFormatSignedJWT, JWT: NewPlaintextSecret(jwt)}
}

func jwtOfLength(t *testing.T, length int) string {
	t.Helper()
	exp := itoa(credentialTestNow.Add(time.Hour).Unix())
	for _, kid := range []string{"k", "kk", "kkk", "kkkk"} {
		estimate := (length - len(signTestJWT(t, kid, `{"exp":`+exp+`,"p":""}`))) * 3 / 4
		for padding := max(0, estimate-3); padding <= estimate+3; padding++ {
			if token := signTestJWT(t, kid, `{"exp":`+exp+`,"p":"`+strings.Repeat("p", padding)+`"}`); len(token) == length {
				return token
			}
		}
	}
	t.Fatalf("cannot build a JWT of %d bytes", length)
	return ""
}

func itoa(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestValidateServiceAccountCredential_Rules(t *testing.T) {
	future := credentialTestNow.Add(time.Hour).Unix()
	valid := signTestJWT(t, "2026-11", t1Payload(future))
	validSegments := strings.Split(valid, ".")
	claims := segment(t1Payload(future))

	tests := []struct {
		name    string
		input   ServiceAccountCredentialInput
		reason  CredentialRejectionReason
		message string
	}{
		{"unknown format", ServiceAccountCredentialInput{Format: "private_key", JWT: NewPlaintextSecret(valid)}, CredentialRejectionUnsupportedFormat, "credential.format must be signed_jwt"},
		{"empty format", ServiceAccountCredentialInput{JWT: NewPlaintextSecret(valid)}, CredentialRejectionUnsupportedFormat, "credential.format must be signed_jwt"},
		{"format checked before use", ServiceAccountCredentialInput{Format: "private_key", Use: CredentialUseAssertion}, CredentialRejectionUnsupportedFormat, "credential.format must be signed_jwt"},
		{"use assertion", ServiceAccountCredentialInput{Format: CredentialFormatSignedJWT, Use: CredentialUseAssertion, JWT: NewPlaintextSecret(valid)}, CredentialRejectionUnsupportedUse, "credential.use must be access_token"},
		{"unknown use", ServiceAccountCredentialInput{Format: CredentialFormatSignedJWT, Use: "refresh_token", JWT: NewPlaintextSecret(valid)}, CredentialRejectionUnsupportedUse, "credential.use must be access_token"},
		{"use checked before jwt", ServiceAccountCredentialInput{Format: CredentialFormatSignedJWT, Use: CredentialUseAssertion}, CredentialRejectionUnsupportedUse, "credential.use must be access_token"},
		{"missing jwt", ServiceAccountCredentialInput{Format: CredentialFormatSignedJWT}, CredentialRejectionMissingJWT, "credential.jwt is required"},
		{"empty jwt", signedInput(""), CredentialRejectionMissingJWT, "credential.jwt is required"},
		{"16385 bytes refused before parsing", signedInput(strings.Repeat("x", ServiceAccountJWTMaxBytes+1)), CredentialRejectionTooLarge, "credential.jwt must be at most 16384 bytes"},
		{"16385-byte signed token", signedInput(jwtOfLength(t, ServiceAccountJWTMaxBytes+1)), CredentialRejectionTooLarge, "credential.jwt must be at most 16384 bytes"},
		{"two segments", signedInput(validSegments[0] + "." + validSegments[1]), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"five segments", signedInput(valid + "." + validSegments[1] + "." + validSegments[2]), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"JSON serialization", signedInput(`{"payload":"` + claims + `"}`), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"empty payload", signedInput(validSegments[0] + ".." + validSegments[2]), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"payload not an object", signedInput(signTestJWT(t, "k", `["read"]`)), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"header not an object", signedInput(segment(`["ES256"]`) + "." + claims + "." + validSegments[2]), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"undecodable segment", signedInput("!!!." + claims + "." + validSegments[2]), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"alg unknown to the library", signedInput(segment(`{"alg":"XYZ"}`) + "." + claims + "." + validSegments[2]), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"alg NONE", signedInput(segment(`{"alg":"NONE"}`) + "." + claims + "."), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"alg None", signedInput(segment(`{"alg":"None"}`) + "." + claims + "."), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"leading space", signedInput(" " + valid), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"trailing space", signedInput(valid + " "), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"boolean exp", signedInput(signTestJWT(t, "k", `{"exp":true}`)), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"exp beyond the date range", signedInput(signTestJWT(t, "k", `{"exp":1e300}`)), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"numeric sub", signedInput(signTestJWT(t, "k", `{"sub":5}`)), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"payload a nested compact JWS", signedInput(signTestJWT(t, "outer", signTestJWT(t, "inner", t1Payload(future)))), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"payload a nested JWS of an unsigned token", signedInput(signTestJWT(t, "outer", segment(`{"alg":"none"}`)+"."+claims+".")), CredentialRejectionMalformed, "credential.jwt must be a compact JWS with valid JWT claims"},
		{"alg none", signedInput(segment(`{"alg":"none"}`) + "." + claims + "."), CredentialRejectionUnsigned, "credential.jwt must be a signed compact JWS"},
		{"no alg", signedInput(segment(`{"typ":"JWT"}`) + "." + claims + "." + validSegments[2]), CredentialRejectionUnsigned, "credential.jwt must be a signed compact JWS"},
		{"exp before the request time", signedInput(signTestJWT(t, "k", t1Payload(credentialTestNow.Unix()-1))), CredentialRejectionExpired, "credential.jwt is expired"},
		{"exp at the request time", signedInput(signTestJWT(t, "k", t1Payload(credentialTestNow.Unix()))), CredentialRejectionExpired, "credential.jwt is expired"},
		{"negative exp", signedInput(signTestJWT(t, "k", `{"exp":-5}`)), CredentialRejectionExpired, "credential.jwt is expired"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validated, err := ValidateServiceAccountCredential(tc.input, credentialTestNow)

			require.Error(t, err)
			assert.Nil(t, validated)
			var rejection *CredentialValidationError
			require.True(t, errors.As(err, &rejection))
			assert.Equal(t, tc.reason, rejection.Reason)
			assert.Equal(t, tc.message, err.Error())
			if submitted, plainErr := tc.input.JWT.GetPlaintext(); plainErr == nil {
				assertNoJWTMaterial(t, err.Error(), submitted)
			}
		})
	}
}

func assertNoJWTMaterial(t *testing.T, text, jwt string) {
	t.Helper()
	for _, part := range append([]string{jwt}, strings.Split(jwt, ".")...) {
		if strings.TrimSpace(part) != "" {
			assert.NotContains(t, text, strings.TrimSpace(part))
		}
	}
}

func TestValidateServiceAccountCredential_Accepts(t *testing.T) {
	future := credentialTestNow.Add(time.Hour).Unix()
	valid := signTestJWT(t, "2026-11", t1Payload(future))
	segments := strings.Split(valid, ".")
	padded := segments[0] + "." + base64.URLEncoding.EncodeToString([]byte(t1Payload(future))) + "." + segments[2]

	tests := []struct {
		name    string
		input   ServiceAccountCredentialInput
		wantExp *int64
	}{
		{"exp one second after the request time", signedInput(signTestJWT(t, "k", t1Payload(credentialTestNow.Unix()+1))), new(credentialTestNow.Unix() + 1)},
		{"no exp", signedInput(signTestJWT(t, "k", `{"iss":"https://idp.example.com"}`)), nil},
		{"explicit use access_token", ServiceAccountCredentialInput{Format: CredentialFormatSignedJWT, Use: CredentialUseAccessToken, JWT: NewPlaintextSecret(valid)}, &future},
		{"exactly 16384 bytes", signedInput(jwtOfLength(t, ServiceAccountJWTMaxBytes)), &future},
		{"trailing line break", signedInput(valid + "\n"), &future},
		{"padded base64", signedInput(padded), &future},
		{"exp as numeric string", signedInput(signTestJWT(t, "k", `{"exp":"`+itoa(future)+`"}`)), &future},
		{"fractional exp in whole seconds", signedInput(signTestJWT(t, "k", `{"exp":`+itoa(future)+`.7}`)), &future},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validated, err := ValidateServiceAccountCredential(tc.input, credentialTestNow)

			require.NoError(t, err)
			submitted, _ := tc.input.JWT.GetPlaintext()
			assert.Equal(t, tc.wantExp, validated.Summary.Exp)
			if tc.wantExp == nil {
				assert.Nil(t, validated.ExpiresAt)
			} else {
				require.NotNil(t, validated.ExpiresAt)
				assert.Equal(t, time.Unix(*tc.wantExp, 0).UTC(), *validated.ExpiresAt)
			}
			assert.Equal(t, CredentialFingerprint(submitted), validated.Summary.Fingerprint)
			document, err := validated.Document.GetPlaintext()
			require.NoError(t, err)
			var decoded map[string]string
			require.NoError(t, json.Unmarshal([]byte(document), &decoded))
			assert.Equal(t, map[string]string{"format": "signed_jwt", "use": "access_token", "jwt": submitted}, decoded, "stored exactly as submitted")
		})
	}
}

func TestValidateServiceAccountCredential_Summary(t *testing.T) {
	exp := credentialTestNow.Add(36 * 24 * time.Hour).Unix()
	token := signTestJWT(t, "2026-11", t1Payload(exp))

	validated, err := ValidateServiceAccountCredential(signedInput(token), credentialTestNow)

	require.NoError(t, err)
	assert.Equal(t, CredentialSummary{
		Format: CredentialFormatSignedJWT, Use: CredentialUseAccessToken,
		Iss: new("https://idp.example.com"), Sub: new("triage-bot"), Kid: new("2026-11"),
		Exp: &exp, Fingerprint: CredentialFingerprint(token),
	}, validated.Summary)

	bare, err := ValidateServiceAccountCredential(signedInput(signTestJWT(t, "", `{"aud":"x"}`)), credentialTestNow)
	require.NoError(t, err)
	assert.Equal(t, CredentialSummary{Format: CredentialFormatSignedJWT, Use: CredentialUseAccessToken, Fingerprint: bare.Summary.Fingerprint}, bare.Summary)
	encoded, err := json.Marshal(bare.Summary)
	require.NoError(t, err)
	assert.JSONEq(t, `{"format":"signed_jwt","use":"access_token","fingerprint":"`+bare.Summary.Fingerprint+`"}`, string(encoded))
}

func TestCredentialFingerprint(t *testing.T) {
	first := signTestJWT(t, "a", `{"exp":1}`)
	second := signTestJWT(t, "a", `{"exp":1}`)

	assert.Equal(t, CredentialFingerprint(first), CredentialFingerprint(first))
	assert.NotEqual(t, CredentialFingerprint(first), CredentialFingerprint(second))
	assert.Regexp(t, regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`), CredentialFingerprint(first))
	digest := sha256.Sum256([]byte(first))
	assert.Equal(t, "SHA256:"+base64.RawStdEncoding.EncodeToString(digest[:]), CredentialFingerprint(first))
	assertNoJWTMaterial(t, CredentialFingerprint(first), first)
}
