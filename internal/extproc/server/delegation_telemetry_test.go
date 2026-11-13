package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/delegation"
)

type observedDelegationResolver struct {
	hit    bool
	err    error
	result *delegation.Result
}

func (r observedDelegationResolver) Resolve(context.Context, string, string) (delegation.Result, bool, error) {
	if r.result != nil {
		return *r.result, r.hit, r.err
	}
	return delegation.Result{Deadline: time.Now().Add(time.Minute), Context: delegation.Context{Principal: "context-principal-marker"}}, r.hit, r.err
}
func (observedDelegationResolver) Shutdown() {}

type permitBindingExchanger struct {
	safetyExchanger
	bindingErr error
}

func (e *permitBindingExchanger) ExchangeBound(context.Context, string, string, *ExchangeResult, TokenBinding) (ExchangeResult, error) {
	return ExchangeResult{Token: "exchanged-token-secret-sentinel"}, e.bindingErr
}

type permitAuthorizer struct{}

func (permitAuthorizer) Evaluate(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
	return &authorization.OPADecision{Action: authorization.ActionAllow}, nil
}
func (permitAuthorizer) Stop(context.Context) {}

func TestDelegationUnavailableSpansAndLogs(t *testing.T) {
	for _, headerOnly := range []bool{false, true} {
		for _, failure := range []string{"unavailable", "permit expired", "binding conflict", "invalid grant timestamp"} {
			path := "body"
			if headerOnly {
				path = "header-only"
			}
			t.Run(path+"/"+failure, func(t *testing.T) {
				recorder, reader := installSafetyTelemetry(t)
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				now := time.Now()
				permit := delegation.Result{Deadline: now.Add(time.Minute), Context: delegation.Context{
					Principal: "context-principal-marker", Grant: delegation.Grant{UpdatedAt: "2026-10-26T09:59:00Z"},
				}}
				resolver := observedDelegationResolver{result: &permit}
				exchanger := &permitBindingExchanger{}
				wantKind := KindUnavailable
				wantEvent := "extproc.delegation_context.unavailable"
				wantBody := delegationUnavailableBody
				switch failure {
				case "unavailable":
					resolver.err = errors.New("dependency-cause-marker")
				case "permit expired":
					permit.Deadline = now
					wantEvent = "extproc.delegation_context.permit_expired"
				case "binding conflict":
					exchanger.bindingErr = ErrTokenBindingConflict
					wantBody = delegationBindingUnavailableBody
				case "invalid grant timestamp":
					permit.Context.Grant.UpdatedAt = "grant-timestamp-marker"
					wantKind = KindInvalidResponse
				}
				cfg := &extprocconfig.Config{}
				cfg.Telemetry.Enabled = true
				cfg.Telemetry.Traces.Enabled = true
				s := NewServerWithAuthorizer(cfg, exchanger, permitAuthorizer{}, logger).WithDelegationResolver(resolver)
				s.now = func() time.Time { return now }
				method := "POST"
				if headerOnly {
					method = "GET"
				}
				headers := &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{Headers: []*corev3.HeaderValue{{Key: ":method", RawValue: []byte(method)}}}}
				req := safetyRequest("subject-token-marker", "https://resource.example/private-marker", headers)
				response, state := s.processRequestHeadersOPA(context.Background(), headerOnly, req, headers)
				if headerOnly {
					require.NotNil(t, state)
					response = s.processHeadersOnlyOPA(state.requestContext, state)
				}
				require.NotNil(t, response.GetImmediateResponse())
				assert.Equal(t, int32(503), int32(response.GetImmediateResponse().GetStatus().GetCode()))
				assert.JSONEq(t, wantBody, string(response.GetImmediateResponse().Body))
				spans := recorder.Ended()
				require.Len(t, spans, 1)
				span := spans[0]
				assert.Equal(t, "extproc.token_exchange", span.Name())
				assert.Equal(t, trace.SpanKindServer, span.SpanKind())
				assert.Equal(t, otelcodes.Error, span.Status().Code)
				attributes := map[string]string{}
				for _, attr := range span.Attributes() {
					attributes[string(attr.Key)] = attr.Value.AsString()
				}
				assert.Equal(t, string(wantKind), attributes["error.type"])
				var record map[string]any
				require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
				assert.Equal(t, "WARN", record["level"])
				assert.Equal(t, wantEvent, record["event"])
				assertSafetyTelemetry(t, recorder, reader, logs.String(), "subject-token-marker", "exchanged-token-secret-sentinel", "context-principal-marker", "dependency-cause-marker", "private-marker", "grant-timestamp-marker")
			})
		}
	}
}

func TestDelegationExpiredLog(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	now := time.Now()
	s := &Server{now: func() time.Time { return now }}
	response, expired := s.delegationExpired(context.Background(), logger, &delegation.Result{Deadline: now})
	require.True(t, expired)
	require.NotNil(t, response)
	var record map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
	assert.Equal(t, "extproc.delegation_context.permit_expired", record["event"])
	assert.Equal(t, "delegation_unavailable", record["token_exchange.failure_detail"])
}

func TestDelegationLogs_ContainOnlyBoundedMetadata(t *testing.T) {
	tests := []struct {
		name            string
		hit             bool
		err             error
		outcome, detail string
	}{
		{"permit miss", false, nil, "success", ""},
		{"permit hit", true, nil, "success", ""},
		{"deny", false, &delegation.DeniedError{Code: "grant_missing", ConsentURL: "https://broker.example/consent?private-marker"}, "authorization_denied", "delegation_denied"},
		{"unavailable", false, errors.New("dependency-cause-marker subject-token-marker assertion-marker provider-token-marker"), "infrastructure_error", "delegation_unavailable"},
		{"invalid response", false, &delegation.Error{Category: delegation.CategoryInvalidResponse}, "infrastructure_error", "delegation_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			s := &Server{delegationResolver: observedDelegationResolver{hit: tt.hit, err: tt.err}}
			_, err := s.resolveDelegation(context.Background(), logger, "subject-token-marker", "https://resource.example/private-marker")
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
			assert.Equal(t, "delegation_resolution", record["operation"])
			cache := "miss"
			if tt.hit {
				cache = "hit"
			}
			assert.Equal(t, cache, record["delegation.cache"])
			assert.Equal(t, tt.outcome, record["token_exchange.outcome"])
			if tt.detail != "" {
				assert.Equal(t, tt.detail, record["token_exchange.failure_detail"])
				event := "extproc.delegation_context.unavailable"
				if tt.detail == "delegation_denied" {
					event = "extproc.delegation_context.denied"
				}
				assert.Equal(t, event, record["event"])
				if tt.detail == "delegation_unavailable" {
					category := delegation.CategoryUnavailable
					if tt.name == "invalid response" {
						category = delegation.CategoryInvalidResponse
					}
					assert.Equal(t, category, record["error_kind"])
					assert.NotContains(t, record, "delegation.error")
				}
			}
			for _, sensitive := range []string{"subject-token-marker", "assertion-marker", "provider-token-marker", "context-principal-marker", "dependency-cause-marker", "private-marker"} {
				assert.NotContains(t, logs.String(), sensitive)
			}
		})
	}
}
