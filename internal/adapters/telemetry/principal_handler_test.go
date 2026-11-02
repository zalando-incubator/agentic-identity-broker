package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
)

func redactPolicy(principal string) string {
	return "redacted:" + principal
}

func TestPrincipalPolicyHandler_RendersTopLevelPrincipalAttribute(t *testing.T) {
	t.Parallel()

	base := newCaptureContextHandler()
	logger := slog.New(newPrincipalPolicyHandler(base, redactPolicy))

	logger.Info("grant created", "owner", id.NewPrincipal("alice@example.com"), "agent_id", "agent-1")

	records := *base.records
	require.Len(t, records, 1)
	assert.Equal(t, "redacted:alice@example.com", records[0].attrs["owner"],
		"any attribute holding an id.Principal must be rendered through the policy, whatever its key")
	assert.Equal(t, "agent-1", records[0].attrs["agent_id"])
}

func TestPrincipalPolicyHandler_RendersPrincipalInsideGroups(t *testing.T) {
	t.Parallel()

	base := newCaptureContextHandler()
	logger := slog.New(newPrincipalPolicyHandler(base, redactPolicy))

	logger.Info("nested",
		slog.Group("grant",
			slog.Any("owner", id.NewPrincipal("alice@example.com")),
			slog.Group("audit", slog.String("principal", "bob@example.com"), slog.String("reason", "revoked")),
		),
	)

	records := *base.records
	require.Len(t, records, 1)
	group, ok := records[0].attrs["grant"].([]slog.Attr)
	require.True(t, ok, "group must stay a group")
	rendered := flattenAttrs("", group)
	assert.Equal(t, "redacted:alice@example.com", rendered["owner"])
	assert.Equal(t, "redacted:bob@example.com", rendered["audit.principal"])
	assert.Equal(t, "revoked", rendered["audit.reason"])
}

func TestPrincipalPolicyHandler_RendersWithAttrs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(newPrincipalPolicyHandler(slog.NewJSONHandler(&buf, nil), redactPolicy)).
		With("owner", id.NewPrincipal("alice@example.com"), "actor", "carol@example.com").
		WithGroup("request").
		With("operator_principal", "op@example.com")

	logger.Info("bound", "method", "GET")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, "redacted:alice@example.com", decoded["owner"])
	assert.Equal(t, "redacted:carol@example.com", decoded["actor"])
	assert.Equal(t, map[string]any{"operator_principal": "redacted:op@example.com", "method": "GET"}, decoded["request"])
}

func TestPrincipalPolicyHandler_RendersReservedStringKeys(t *testing.T) {
	t.Parallel()

	base := newCaptureContextHandler()
	logger := slog.New(newPrincipalPolicyHandler(base, redactPolicy))

	logger.Info("strings",
		"principal", "alice@example.com",
		"operator_principal", "op@example.com",
		"actor", "carol@example.com",
		"calling_peer", "gateway",
		"principal_count", 3,
	)

	records := *base.records
	require.Len(t, records, 1)
	assert.Equal(t, "redacted:alice@example.com", records[0].attrs["principal"])
	assert.Equal(t, "redacted:op@example.com", records[0].attrs["operator_principal"])
	assert.Equal(t, "redacted:carol@example.com", records[0].attrs["actor"])
	assert.Equal(t, "gateway", records[0].attrs["calling_peer"], "non-reserved keys stay unchanged")
	assert.Equal(t, int64(3), records[0].attrs["principal_count"])
}

func TestPrincipalPolicyHandler_ReservedKeysOnlyApplyToStrings(t *testing.T) {
	t.Parallel()

	base := newCaptureContextHandler()
	logger := slog.New(newPrincipalPolicyHandler(base, redactPolicy))

	logger.Info("non-string", "principal", 42)

	records := *base.records
	require.Len(t, records, 1)
	assert.Equal(t, int64(42), records[0].attrs["principal"])
}

func TestPrincipalPolicyHandler_RendersActorFromContextHandler(t *testing.T) {
	t.Parallel()

	base := newCaptureContextHandler()
	logger := slog.New(NewContextHandler(newPrincipalPolicyHandler(base, redactPolicy)))
	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
		TraceID: "0123456789abcdef0123456789abcdef",
		Actor:   id.NewPrincipal("alice@example.com"),
	})

	logger.InfoContext(ctx, "request handled")

	records := *base.records
	require.Len(t, records, 1)
	assert.Equal(t, "redacted:alice@example.com", records[0].attrs["actor"])
}

func TestPrincipalPolicyHandler_IdentityPolicyKeepsOutputUnchanged(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		newBase func(*bytes.Buffer) slog.Handler
	}{
		{"text", func(buf *bytes.Buffer) slog.Handler { return slog.NewTextHandler(buf, nil) }},
		{"json", func(buf *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(buf, nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			emit := func(handler slog.Handler) {
				slog.New(handler).With("operator_principal", "op@example.com").Info("identity",
					"principal", id.NewPrincipal("alice@example.com"),
					"actor", "carol@example.com",
					slog.Group("grant", slog.Any("owner", id.NewPrincipal("bob@example.com"))),
				)
			}

			var plainBuf, policyBuf bytes.Buffer
			emit(tc.newBase(&plainBuf))
			emit(NewPrincipalPolicyHandler(tc.newBase(&policyBuf)))

			withPolicy := removeTime(t, policyBuf.String())
			assert.Equal(t, removeTime(t, plainBuf.String()), withPolicy)
			assert.Contains(t, withPolicy, "alice@example.com")
			assert.Contains(t, withPolicy, "bob@example.com")
			assert.Contains(t, withPolicy, "op@example.com")
		})
	}
}

func TestPrincipalPolicyHandler_RedactsTextAndJSONOutput(t *testing.T) {
	t.Parallel()

	policy := func(string) string { return "[principal]" }
	var textBuf, jsonBuf bytes.Buffer
	text := slog.New(newPrincipalPolicyHandler(slog.NewTextHandler(&textBuf, nil), policy))
	jsonLogger := slog.New(newPrincipalPolicyHandler(slog.NewJSONHandler(&jsonBuf, nil), policy))
	for _, logger := range []*slog.Logger{text, jsonLogger} {
		logger.With("actor", "carol@example.com").Info("redacted",
			"principal", id.NewPrincipal("alice@example.com"),
			slog.Group("grant", slog.Any("owner", id.NewPrincipal("bob@example.com"))),
		)
	}

	assert.NotContains(t, textBuf.String(), "example.com")
	assert.Contains(t, textBuf.String(), "principal=[principal]")
	assert.Contains(t, textBuf.String(), "grant.owner=[principal]")
	assert.Contains(t, textBuf.String(), "actor=[principal]")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(jsonBuf.Bytes(), &decoded))
	assert.Equal(t, "[principal]", decoded["principal"])
	assert.Equal(t, "[principal]", decoded["actor"])
	assert.Equal(t, map[string]any{"owner": "[principal]"}, decoded["grant"])
}

func removeTime(t *testing.T, line string) string {
	t.Helper()
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "{") {
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &decoded))
		delete(decoded, "time")
		normalized, err := json.Marshal(decoded)
		require.NoError(t, err)
		return string(normalized)
	}
	_, rest, found := strings.Cut(line, " ")
	require.True(t, found)
	return rest
}

func flattenAttrs(prefix string, attrs []slog.Attr) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, attr := range attrs {
		key := prefix + attr.Key
		if attr.Value.Kind() == slog.KindGroup {
			for k, v := range flattenAttrs(key+".", attr.Value.Group()) {
				out[k] = v
			}
			continue
		}
		out[key] = attr.Value.Resolve().Any()
	}
	return out
}
