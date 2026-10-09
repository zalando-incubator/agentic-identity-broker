package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

func TestServerRejectsAmbiguousMCPBeforeAuthorization(t *testing.T) {
	for _, body := range []string{
		`{"jsonrpc":"2.0","method":"tools/call","Method":"initialize","params":{"name":"safe"}}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dangerous","arguments":{"path":"/prod"},"Name":"safe","Arguments":{"path":"/tmp"}}}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"deploy","Arguments":{"path":"/prod"}}}`,
		`{"jsonrpc":"2.0","method":"initialize","Params":{"capabilities":{"tools":true}}}`,
		`[{"jsonrpc":"2.0","method":"initialize","params":{}},{"jsonrpc":"2.0","method":"initialize","Params":{}}]`,
		`[{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe"}},{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dangerous","Name":"safe"}}]`,
	} {
		t.Run(body, func(t *testing.T) {
			authorizerCalls := 0
			auth := &mockAuthorizer{evaluateFunc: func(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
				authorizerCalls++
				return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
			}}
			exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
				return server.ExchangeResult{Token: "exchanged", Principal: "alice", AgentID: "agent"}, nil
			}}
			client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
			defer cleanup()

			_, bodyResp := sendHeadersThenBody(t, client, map[string]string{":method": "POST"}, []byte(body))
			require.NotNil(t, bodyResp.GetImmediateResponse())
			assert.Equal(t, httpv3.StatusCode_Forbidden, bodyResp.GetImmediateResponse().Status.Code)
			assert.Zero(t, authorizerCalls)
		})
	}
}

func TestServerForwardsOnlyApprovedMCPView(t *testing.T) {
	permanent := "permanent"
	approvedAt := time.Now()
	cache := approval.NewCache(time.Minute, time.Minute)
	cache.Replace([]approval.Pair{{
		Identity: approval.Identity{Principal: "alice", AgentID: "agent"},
		Approvals: []approval.Record{{
			ID: "approval-1", ToolPattern: "deploy", ParamsPattern: map[string]string{},
			Status: "approved", Persistence: &permanent, ApprovedAt: &approvedAt,
		}},
	}}, "etag-1")
	broker := &mockApprovalBroker{
		readFunc: func(context.Context, string, []string) ([]approval.Pair, string, error) {
			t.Error("approved invocation must match the cached approval")
			return nil, "", nil
		},
	}
	var opaBody any
	auth := &mockAuthorizer{evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
		mcp, ok := input["mcp"].(map[string]any)
		require.True(t, ok, "policy must receive the decoded MCP map")
		assert.Equal(t, "deploy", mcp["tool_name"])
		assert.Equal(t, map[string]any{"path": "/prod"}, mcp["arguments"])
		opaBody = input["parsed_body"]
		return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
	}}
	exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
		return server.ExchangeResult{Token: "exchanged", Principal: "alice", AgentID: "agent"}, nil
	}}
	client, cleanup := startTestServerWithAuthorizerConfigAndGate(t, testConfig(), exchanger, auth, approval.NewGate(cache, broker))
	defer cleanup()

	body := []byte(` { "jsonrpc" : "2.0", "method" : "tools/call", "id": 1, "params": {"name":"deploy", "arguments":{"path":"/tmp", "path":"/prod"}}, "extension":{"source":"client"} } `)
	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{":method": "POST"}, body)
	require.Nil(t, bodyResp.GetImmediateResponse())
	forwarded := bodyResp.GetRequestBody().GetResponse().GetBodyMutation().GetStreamedResponse()
	require.NotNil(t, forwarded)
	assert.True(t, forwarded.EndOfStream)
	assert.NotEqual(t, body, forwarded.Body)
	assert.Equal(t, 1, bytes.Count(forwarded.Body, []byte(`"path"`)), "approved request must not forward duplicate argument keys")

	var upstreamBody any
	decoder := json.NewDecoder(bytes.NewReader(forwarded.Body))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&upstreamBody))
	assert.Equal(t, opaBody, upstreamBody)
	assert.JSONEq(t, `{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"deploy","arguments":{"path":"/prod"}},"extension":{"source":"client"}}`, string(forwarded.Body))
}

func TestServerPreservesOnceApprovalWhenSerializedBodyIsTooLarge(t *testing.T) {
	once := "once"
	approvedAt := time.Now()
	cache := approval.NewCache(time.Minute, time.Minute)
	cache.Replace([]approval.Pair{{
		Identity: approval.Identity{Principal: "alice", AgentID: "agent"},
		Approvals: []approval.Record{{
			ID: "approval-1", ToolPattern: "deploy", ParamsPattern: map[string]string{},
			Status: "approved", Persistence: &once, ApprovedAt: &approvedAt,
		}},
	}}, "etag-1")
	var consumes atomic.Int32
	broker := &mockApprovalBroker{
		readFunc: func(context.Context, string, []string) ([]approval.Pair, string, error) {
			t.Error("unspent cached approval must cover the retry")
			return nil, "", nil
		},
		createFunc: func(context.Context, string, approval.CreateRequest) (string, error) {
			t.Error("retry must not require another approval")
			return "", nil
		},
		consumeFunc: func(context.Context, string, string) error {
			consumes.Add(1)
			return nil
		},
	}
	auth := &mockAuthorizer{evaluateFunc: func(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
		return &authorization.OPADecision{Action: authorization.ActionApprovalRequired}, nil
	}}
	exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
		return server.ExchangeResult{Token: "exchanged", Principal: "alice", AgentID: "agent"}, nil
	}}
	body := []byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"deploy","arguments":{}},"extension":"` + strings.Repeat("\u2028", 30) + `"}`)
	cfg := testConfig()
	cfg.Authorization.MaxBodySize = len(body)
	client, cleanup := startTestServerWithAuthorizerConfigAndGate(t, cfg, exchanger, auth, approval.NewGate(cache, broker))
	defer cleanup()

	_, rejected := sendHeadersThenBody(t, client, map[string]string{":method": "POST"}, body)
	require.NotNil(t, rejected.GetImmediateResponse())
	assert.Contains(t, string(rejected.GetImmediateResponse().Body), "request_too_large")
	assert.Zero(t, consumes.Load(), "serialization rejection must not spend the approval")

	_, retry := sendHeadersThenBody(t, client, map[string]string{":method": "POST"}, []byte(`{"jsonrpc":"2.0","method":"tools/call","id":1,"params":{"name":"deploy","arguments":{}}}`))
	require.NotNil(t, retry.GetRequestBody().GetResponse().GetBodyMutation().GetStreamedResponse())
	assert.Equal(t, int32(1), consumes.Load(), "only the forwarded retry spends the approval")
}
