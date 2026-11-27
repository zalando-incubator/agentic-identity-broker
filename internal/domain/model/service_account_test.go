package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// policyHandler stands in for the principal policy of coding-standards/LOGGING.md section 5: it
// renders every id.Principal attribute, also inside groups, as "policy(<value>)".
type policyHandler struct{ next slog.Handler }

func (h policyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h policyHandler) Handle(ctx context.Context, record slog.Record) error {
	rendered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		rendered.AddAttrs(renderPrincipals(attr))
		return true
	})
	return h.next.Handle(ctx, rendered)
}

func (h policyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return policyHandler{h.next.WithAttrs(attrs)}
}

func (h policyHandler) WithGroup(name string) slog.Handler {
	return policyHandler{h.next.WithGroup(name)}
}

func renderPrincipals(attr slog.Attr) slog.Attr {
	if principal, ok := attr.Value.Any().(id.Principal); ok && attr.Value.Kind() == slog.KindLogValuer {
		return slog.String(attr.Key, "policy("+principal.String()+")")
	}
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		group := value.Group()
		rendered := make([]slog.Attr, len(group))
		for i, member := range group {
			rendered[i] = renderPrincipals(member)
		}
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(rendered...)}
	}
	return slog.Attr{Key: attr.Key, Value: value}
}

func logRecord(t *testing.T, args ...any) map[string]any {
	t.Helper()
	var buffer bytes.Buffer
	logger := slog.New(policyHandler{slog.NewJSONHandler(&buffer, nil)})
	logger.InfoContext(context.Background(), "test record", args...)
	var record map[string]any
	require.NoError(t, json.Unmarshal(buffer.Bytes(), &record))
	return record
}

func TestValidateServiceAccountCanonicalID(t *testing.T) {
	assert.NoError(t, ValidateServiceAccountCanonicalID("platform-triage-bot"))
	for _, invalid := range []string{"", "platform triage bot", strings.Repeat("a", 129), "990e8400-e29b-41d4-a716-446655440031"} {
		err := ValidateServiceAccountCanonicalID(invalid)
		var validation *ServiceAccountValidationError
		require.True(t, errors.As(err, &validation), invalid)
		assert.True(t, strings.HasPrefix(validation.Message, "canonical_id must "), validation.Message)
	}
}

func TestValidateServiceAccountKind(t *testing.T) {
	assert.NoError(t, ValidateServiceAccountKind(ServiceAccountKindJWTBearer))
	for _, kind := range []ServiceAccountKind{"", "client_credentials", "JWT_BEARER"} {
		err := ValidateServiceAccountKind(kind)
		require.Error(t, err)
		assert.Equal(t, "kind must be jwt_bearer", err.Error())
		var validation *ServiceAccountValidationError
		assert.True(t, errors.As(err, &validation))
	}
}

func TestNormalizeServiceAccountScopes(t *testing.T) {
	normalized, err := NormalizeServiceAccountScopes([]string{"write", "read", "admin"})
	require.NoError(t, err)
	assert.Equal(t, []string{"admin", "read", "write"}, normalized)

	input := []string{"b", "a"}
	_, err = NormalizeServiceAccountScopes(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "a"}, input, "the input is not reordered in place")

	for name, scopes := range map[string][]string{"nil": nil, "empty": {}, "empty value": {"read", ""}, "duplicate": {"read", "read"}} {
		_, err := NormalizeServiceAccountScopes(scopes)
		var validation *ServiceAccountValidationError
		require.True(t, errors.As(err, &validation), name)
		assert.Equal(t, "scopes must be a non-empty list of unique, non-empty values", validation.Message, name)
	}
}

func TestServiceAccount_StatusAt(t *testing.T) {
	expiry := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	account := &ServiceAccount{CredentialExpiresAt: &expiry}

	status, reason := account.StatusAt(expiry.Add(-time.Nanosecond))
	assert.Equal(t, ServiceAccountStatusReady, status)
	assert.Nil(t, reason)

	for _, at := range []time.Time{expiry, expiry.Add(5 * time.Second), expiry.Add(365 * 24 * time.Hour)} {
		status, reason := account.StatusAt(at)
		assert.Equal(t, ServiceAccountStatusFailed, status)
		require.NotNil(t, reason)
		assert.Equal(t, ServiceAccountFailureCredentialExpired, *reason)
	}

	withoutExpiry := &ServiceAccount{}
	for _, at := range []time.Time{{}, expiry, expiry.AddDate(100, 0, 0)} {
		status, reason := withoutExpiry.StatusAt(at)
		assert.Equal(t, ServiceAccountStatusReady, status)
		assert.Nil(t, reason)
	}
}

func TestServiceAccount_Narrowing(t *testing.T) {
	account := &ServiceAccount{Kind: ServiceAccountKindJWTBearer, Summary: CredentialSummary{Format: CredentialFormatSignedJWT}}
	assert.Equal(t, NarrowingNone, account.Narrowing())
}

func TestCredentialSummary_LogValue(t *testing.T) {
	long := strings.Repeat("é", 200)
	exp := int64(1798675200)
	summary := CredentialSummary{
		Format: CredentialFormatSignedJWT, Use: CredentialUseAccessToken,
		Iss: &long, Sub: new("triage-bot"), Kid: new("2026-11"), Exp: &exp,
		Fingerprint: "SHA256:fhbwk8g0dDlEiuXzadc7Ytx+R1pSlxRBBNgzBxN+3UQ",
	}

	group := logRecord(t, "credential", summary)["credential"].(map[string]any)

	assert.Equal(t, "signed_jwt", group["format"])
	assert.Equal(t, "access_token", group["use"])
	assert.Equal(t, "policy(triage-bot)", group["sub"], "sub must pass through the principal policy")
	assert.Equal(t, "2026-11", group["kid"])
	assert.Equal(t, float64(exp), group["exp"])
	assert.Equal(t, summary.Fingerprint, group["fingerprint"])
	iss := group["iss"].(string)
	assert.LessOrEqual(t, len(iss), 256)
	assert.True(t, strings.HasPrefix(long, iss))
	assert.Greater(t, len(iss), 250)
	assert.Equal(t, "triage-bot", *summary.Sub, "the summary itself keeps sub as a string")

	longSub := strings.Repeat("s", 300)
	capped := logRecord(t, "credential", CredentialSummary{Format: CredentialFormatSignedJWT, Use: CredentialUseAccessToken, Sub: &longSub, Fingerprint: "SHA256:x"})["credential"].(map[string]any)
	assert.Equal(t, "policy("+strings.Repeat("s", 256)+")", capped["sub"])

	bare := logRecord(t, "credential", CredentialSummary{Format: CredentialFormatSignedJWT, Use: CredentialUseAccessToken, Fingerprint: "SHA256:x"})["credential"].(map[string]any)
	assert.Equal(t, map[string]any{"format": "signed_jwt", "use": "access_token", "fingerprint": "SHA256:x"}, bare)
}

func TestServiceAccountCredentialInput_Redacted(t *testing.T) {
	const jwt = "eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ0cmlhZ2UtYm90In0.c2lnbmF0dXJlLXNlZ21lbnQ"
	input := ServiceAccountCredentialInput{Format: CredentialFormatSignedJWT, Use: CredentialUseAccessToken, JWT: NewPlaintextSecret(jwt)}

	rendered := []string{
		fmt.Sprintf("%v", input), fmt.Sprintf("%+v", input), fmt.Sprintf("%#v", input),
		fmt.Sprintf("%v", &input), input.String(), input.GoString(),
	}
	var buffer bytes.Buffer
	slog.New(slog.NewJSONHandler(&buffer, nil)).InfoContext(context.Background(), "input", "credential", input)
	rendered = append(rendered, buffer.String())
	for _, text := range rendered {
		assertNoJWTMaterial(t, text, jwt)
	}
	assert.Contains(t, buffer.String(), "REDACTED")
}
