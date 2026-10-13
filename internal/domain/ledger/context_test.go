package ledger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func contextTestRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	return registry
}

func contextTestEvent(t *testing.T, registry *Registry, name string, facts model.BusinessEvent) model.BusinessEvent {
	t.Helper()
	facts.OccurredAt = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	if facts.Data == nil {
		facts.Data = map[string]any{}
	}
	event, err := NewService(registry, nil, nil, nil, false).NewEvent(context.Background(), model.BusinessEventTypePrefix+name, facts)
	require.NoError(t, err)
	event.RecordedAt = event.OccurredAt.Add(time.Second)
	return *event
}

func validatedContextWire(t *testing.T, registry *Registry, ctx context.Context, facts model.BusinessEvent) (map[string]any, string) {
	t.Helper()
	event := ContextFacts(ctx, facts)
	_, err := registry.Validate(&event)
	require.NoError(t, err, "enriched facts must satisfy the published event schema")
	encoded, err := json.Marshal(&event)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	return wire, string(encoded)
}

func otelContextWithSpan(t *testing.T, ctx context.Context, traceHex, spanHex string) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex(traceHex)
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex(spanHex)
	require.NoError(t, err)
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	require.True(t, span.IsValid())
	return trace.ContextWithSpanContext(ctx, span)
}

func TestContextFactsKeepsDelegatedCallerAndAuthoritativeReferences(t *testing.T) {
	registry := contextTestRegistry(t)
	subject := id.NewPrincipal("represented-user")
	const caller = "verified-gateway-client"
	gateway := id.NewClientID(caller)
	agent := id.MustParseAgentID("11111111-1111-4111-8111-111111111111")
	service := id.MustParseServiceID("33333333-3333-4333-8333-333333333333")
	grant := id.MustParseGrantID("22222222-2222-4222-8222-222222222222")
	session := id.MustParseSessionID("44444444-4444-4444-8444-444444444444")
	permission := id.MustParsePermissionSetID("55555555-5555-4555-8555-555555555555")
	facts := contextTestEvent(t, registry, "token-exchanged", model.BusinessEvent{
		Subject: &subject, Actor: model.BusinessEventActor{Kind: "gateway", OnBehalfOf: &subject},
		AgentID: agent, GatewayClientID: gateway, ServiceID: service, GrantID: grant, SessionID: session,
		PermissionSetIDs: []id.PermissionSetID{permission},
	})
	facts.SetTrustedSessionCorrelations("mcp/session-approved", "agent session reviewed")
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const spanID = "00f067aa0ba902b7"
	const rawCredential = "raw-ua-Chrome-credential-canary"
	ctx := otelContextWithSpan(t, context.Background(), traceID, spanID)
	ctx = security.WithSecurityContext(ctx, security.NewSecurityContext(security.TransportCapture{
		TraceID: traceID, ClientIP: "2001:db8::1", UserAgent: "curl/8.17 (Bearer " + rawCredential + ")",
		RequestTarget: "/oauth2/token?subject=forged-user&access_token=raw-request-credential",
	}, subject.String(), caller))

	wire, encoded := validatedContextWire(t, registry, ctx, facts)
	actor := wire["actor"].(map[string]any)
	require.Equal(t, subject.String(), wire["subject"])
	require.Equal(t, "gateway", actor["kind"])
	require.Equal(t, caller, actor["id"], "the verified caller, not the represented principal, initiated the exchange")
	require.Equal(t, subject.String(), actor["on_behalf_of"])
	require.Equal(t, gateway.String(), wire["gateway_client_id"])
	require.Equal(t, agent.String(), wire["agent_id"])
	require.Equal(t, service.String(), wire["service_id"])
	require.Equal(t, grant.String(), wire["grant_id"])
	require.Equal(t, session.String(), wire["session_id"])
	require.Equal(t, []any{permission.String()}, wire["permission_set_ids"])
	require.Equal(t, "mcp/session-approved", wire["mcp_session_id"])
	require.Equal(t, "agent session reviewed", wire["agent_session_id"])
	require.Equal(t, traceID, wire["trace_id"])
	require.Equal(t, spanID, wire["span_id"])
	require.Equal(t, map[string]any{"ip": "2001:db8::1", "user_agent": "curl"}, wire["client"])
	require.Equal(t, "The broker completes a token exchange for a receiving agent.", wire["reason_user"])
	require.Equal(t, wire["reason_user"], wire["reason_admin"])
	require.NotContains(t, encoded, rawCredential)
	require.NotContains(t, encoded, "raw-request-credential")
	require.NotContains(t, encoded, "forged-user")
}

func TestContextFactsDoesNotInferDelegationOrGatewayFromPeerName(t *testing.T) {
	registry := contextTestRegistry(t)
	subject := id.NewPrincipal("validated-subject")
	const caller = "gateway-admin-client-name"
	facts := contextTestEvent(t, registry, "token-exchange-denied", model.BusinessEvent{
		Subject: &subject, Actor: model.BusinessEventActor{Kind: "gateway"},
		Data: map[string]any{"reason_code": "authorization_failed"},
	})
	ctx := security.WithSecurityContext(context.Background(), security.NewSecurityContext(
		security.TransportCapture{RequestTarget: "/oauth2/token?gateway_client_id=unverified-gateway"}, subject.String(), caller,
	))
	wire, _ := validatedContextWire(t, registry, ctx, facts)
	actor := wire["actor"].(map[string]any)
	require.Equal(t, subject.String(), wire["subject"])
	require.Equal(t, "gateway", actor["kind"], "caller category belongs to the authenticated workflow")
	require.Equal(t, caller, actor["id"])
	require.Nil(t, actor["on_behalf_of"], "validated identities alone do not establish delegation")
	require.NotContains(t, wire, "gateway_client_id", "the caller name is not a verified gateway association")
	require.Equal(t, "An exchange is refused by authentication or authorization controls.", wire["reason_user"])

	approved := contextTestEvent(t, registry, "approval-requested", model.BusinessEvent{
		Subject: &subject, Actor: model.BusinessEventActor{Kind: "gateway"},
		GatewayClientID: id.NewClientID(caller),
		AgentID:         id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
		ApprovalID:      id.MustParseApprovalID("66666666-6666-4666-8666-666666666666"),
	})
	wire, _ = validatedContextWire(t, registry, ctx, approved)
	actor = wire["actor"].(map[string]any)
	require.Equal(t, "gateway", actor["kind"])
	require.Equal(t, caller, actor["id"])
	require.Nil(t, actor["on_behalf_of"])
	require.Equal(t, caller, wire["gateway_client_id"], "the association came from verified domain facts")
}

func TestContextFactsKeepsEstablishedCallerOverDifferentRequestPeer(t *testing.T) {
	registry := contextTestRegistry(t)
	caller := "verified-initiating-client"
	subject := id.NewPrincipal("represented-user")
	facts := contextTestEvent(t, registry, "token-exchange-denied", model.BusinessEvent{
		Subject: &subject, Actor: model.BusinessEventActor{Kind: "gateway", ID: &caller},
		GatewayClientID: id.NewClientID(caller),
		Data:            map[string]any{"reason_code": "authorization_failed"},
	})
	ctx := security.WithSecurityContext(context.Background(), security.NewSecurityContext(
		security.TransportCapture{}, subject.String(), "different-peer-from-request-context",
	))
	wire, _ := validatedContextWire(t, registry, ctx, facts)
	actor := wire["actor"].(map[string]any)
	require.Equal(t, "gateway", actor["kind"])
	require.Equal(t, caller, actor["id"], "request enrichment must not overwrite a verified domain caller")
	require.Nil(t, actor["on_behalf_of"], "an authenticated assertion alone is not delegated")
	require.Equal(t, caller, wire["gateway_client_id"])
}

func TestContextFactsPreservesPolicyEstablishedSubjectWithoutRecheckingJWT(t *testing.T) {
	registry := contextTestRegistry(t)
	accepted := id.NewPrincipal("policy-bound-chat-user")
	caller := "signed-client-assertion-subject"
	ctx := security.WithSecurityContext(context.Background(), security.NewSecurityContext(
		security.TransportCapture{RequestTarget: "/oauth2/token?subject_token=unverified-claim-other-user"},
		security.AnonymousActor, "",
	))

	granted := contextTestEvent(t, registry, "impersonation-granted", model.BusinessEvent{
		Subject: &accepted, Actor: model.BusinessEventActor{Kind: "gateway", ID: &caller, OnBehalfOf: &accepted},
	})
	wire, encoded := validatedContextWire(t, registry, ctx, granted)
	actor := wire["actor"].(map[string]any)
	require.Equal(t, accepted.String(), wire["subject"], "the domain's accepted-policy result owns subject attribution")
	require.Equal(t, caller, actor["id"])
	require.Equal(t, accepted.String(), actor["on_behalf_of"])
	require.NotContains(t, encoded, "unverified-claim-other-user")

	denied := contextTestEvent(t, registry, "impersonation-denied", model.BusinessEvent{
		Actor: model.BusinessEventActor{Kind: "gateway", ID: &caller},
		Data:  map[string]any{"reason_code": "delegation_missing"},
	})
	wire, _ = validatedContextWire(t, registry, ctx, denied)
	require.Contains(t, wire, "subject")
	require.Nil(t, wire["subject"], "an unsigned assertion alone never establishes a subject")
	actor = wire["actor"].(map[string]any)
	require.Equal(t, caller, actor["id"])
	require.Nil(t, actor["on_behalf_of"])
}

func TestContextFactsUsesOnlyMatchingValidOTelRequestSpan(t *testing.T) {
	registry := contextTestRegistry(t)
	const requestTrace = "4bf92f3577b34da6a3ce929d0e0e4736"
	const differentTrace = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const serverSpan = "00f067aa0ba902b7"
	cases := []struct {
		name, authoritativeTrace, activeSpanTrace, expectedTrace, expectedSpan string
	}{
		{"matching span", requestTrace, requestTrace, requestTrace, serverSpan},
		{"different active trace", requestTrace, differentTrace, requestTrace, ""},
		{"no active span", requestTrace, "", requestTrace, ""},
		{"invalid captured trace", "not-a-trace-id", requestTrace, "", ""},
		{"no request trace", "", requestTrace, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.activeSpanTrace != "" {
				ctx = otelContextWithSpan(t, ctx, tc.activeSpanTrace, serverSpan)
			}
			ctx = security.WithSecurityContext(ctx, security.NewSecurityContext(
				security.TransportCapture{TraceID: tc.authoritativeTrace}, security.AnonymousActor, "",
			))
			facts := contextTestEvent(t, registry, "token-request-failed", model.BusinessEvent{
				Actor: model.BusinessEventActor{Kind: "agent"}, Data: map[string]any{"reason_code": "invalid_request"},
			})
			wire, _ := validatedContextWire(t, registry, ctx, facts)
			if tc.expectedTrace == "" {
				require.NotContains(t, wire, "trace_id")
			} else {
				require.Equal(t, tc.expectedTrace, wire["trace_id"])
			}
			if tc.expectedSpan == "" {
				require.NotContains(t, wire, "span_id")
			} else {
				require.Equal(t, tc.expectedSpan, wire["span_id"])
			}
		})
	}
}

func TestContextFactsUsesNullUnavailableIdentitiesAndNoBackgroundRequestCorrelation(t *testing.T) {
	registry := contextTestRegistry(t)
	for _, tc := range []struct {
		name, eventName, kind string
		data                  map[string]any
	}{
		{"pre-authentication failure", "token-request-failed", "agent", map[string]any{"reason_code": "authentication_failed"}},
		{"administration without an operator", "agent-registered", "admin", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := model.BusinessEvent{Actor: model.BusinessEventActor{Kind: tc.kind}, Data: tc.data}
			if tc.kind == "admin" {
				facts.AgentID = id.MustParseAgentID("11111111-1111-4111-8111-111111111111")
			}
			facts = contextTestEvent(t, registry, tc.eventName, facts)
			ctx := security.WithSecurityContext(context.Background(), security.NewSecurityContext(
				security.TransportCapture{RequestTarget: "/oauth2/token?sub=forged-principal"}, security.AnonymousActor, "",
			))
			wire, _ := validatedContextWire(t, registry, ctx, facts)
			require.Contains(t, wire, "subject")
			require.Nil(t, wire["subject"])
			actor := wire["actor"].(map[string]any)
			require.Equal(t, tc.kind, actor["kind"])
			require.Contains(t, actor, "id")
			require.Nil(t, actor["id"], "anonymous is a slog sentinel, not an authenticated identity")
			require.Nil(t, actor["on_behalf_of"])
		})
	}

	operator := "authenticated-operator"
	admin := contextTestEvent(t, registry, "agent-registered", model.BusinessEvent{
		Actor:   model.BusinessEventActor{Kind: "admin"},
		AgentID: id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
	})
	ctx := security.WithSecurityContext(context.Background(), security.NewSecurityContext(
		security.TransportCapture{}, operator, "",
	))
	wire, _ := validatedContextWire(t, registry, ctx, admin)
	require.Nil(t, wire["subject"], "an administrator is not the subject of a non-user resource event")
	require.Equal(t, operator, wire["actor"].(map[string]any)["id"])
	user := id.NewPrincipal("authenticated-user")
	grant := contextTestEvent(t, registry, "grant-created", model.BusinessEvent{
		Subject: &user, Actor: model.BusinessEventActor{Kind: "user"},
		AgentID: id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
		GrantID: id.MustParseGrantID("22222222-2222-4222-8222-222222222222"),
	})
	ctx = security.WithSecurityContext(context.Background(), security.NewSecurityContext(
		security.TransportCapture{}, user.String(), "",
	))
	wire, _ = validatedContextWire(t, registry, ctx, grant)
	require.Equal(t, user.String(), wire["subject"])
	require.Equal(t, user.String(), wire["actor"].(map[string]any)["id"])

	subject := id.NewPrincipal("expired-grant-owner")
	systemID := "broker-lifecycle"
	background := contextTestEvent(t, registry, "grant-expired", model.BusinessEvent{
		Subject: &subject, Actor: model.BusinessEventActor{Kind: "system", ID: &systemID},
		AgentID: id.MustParseAgentID("11111111-1111-4111-8111-111111111111"),
		GrantID: id.MustParseGrantID("22222222-2222-4222-8222-222222222222"),
	})
	ctx = otelContextWithSpan(t, context.Background(), "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")
	wire, _ = validatedContextWire(t, registry, ctx, background)
	require.Equal(t, subject.String(), wire["subject"])
	require.Equal(t, "system", wire["actor"].(map[string]any)["kind"])
	require.Equal(t, systemID, wire["actor"].(map[string]any)["id"])
	require.NotContains(t, wire, "trace_id", "a background span is not an HTTP request trace")
	require.NotContains(t, wire, "span_id")
	require.NotContains(t, wire, "client")
}

func TestContextFactsNeverCertifiesRawSessionStringsOrUnknownUserAgent(t *testing.T) {
	registry := contextTestRegistry(t)
	const credential = "untrusted-session-credential-canary"
	facts := contextTestEvent(t, registry, "token-request-failed", model.BusinessEvent{
		Actor:        model.BusinessEventActor{Kind: "agent"},
		MCPSessionID: "mcp/" + credential, AgentSessionID: "agent/" + credential,
		Data: map[string]any{"reason_code": "authentication_failed"},
	})
	ctx := security.WithSecurityContext(context.Background(), security.NewSecurityContext(
		security.TransportCapture{UserAgent: "Unknown-Agent/1.0 (" + credential + ")"}, security.AnonymousActor, "",
	))
	wire, encoded := validatedContextWire(t, registry, ctx, facts)
	require.NotContains(t, wire, "mcp_session_id")
	require.NotContains(t, wire, "agent_session_id")
	require.NotContains(t, wire, "client", "unrecognized user agents do not acquire an unsafe raw fallback")
	require.NotContains(t, encoded, credential)
}
