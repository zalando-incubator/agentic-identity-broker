package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

func TestServer_OPA_MCPDecoderRejectsAmbiguousOrNoncanonicalKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"duplicate envelope method", `{"jsonrpc":"2.0","method":"tools/call","method":"initialize","params":{"name":"safe"}}`},
		{"folded envelope method", `{"jsonrpc":"2.0","method":"tools/call","Method":"initialize","params":{"name":"safe"}}`},
		{"folded envelope id", `{"jsonrpc":"2.0","method":"tools/call","id":1,"ID":2,"params":{"name":"safe"}}`},
		{"duplicate params name", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","name":"dangerous"}}`},
		{"folded params name", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","Name":"dangerous"}}`},
		{"folded params arguments", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","arguments":{"path":"/safe"},"Arguments":{"path":"/dangerous"}}}`},
		{"escaped params name", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","\u006eame":"dangerous"}}`},
		{"Unicode fold", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe","K":1,"\u212a":2}}`},
		{"tool arguments alias", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"deploy","Arguments":{"path":"/prod"}}}`},
		{"escaped tool arguments alias", `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"deploy","\u0041rguments":{"path":"/prod"}}}`},
		{"tool name alias", `{"jsonrpc":"2.0","method":"tools/call","params":{"Name":"deploy","arguments":{"path":"/prod"}}}`},
		{"non-tool params alias", `{"jsonrpc":"2.0","method":"initialize","Params":{"capabilities":{"tools":true}}}`},
		{"request id alias", `{"jsonrpc":"2.0","method":"initialize","ID":42,"params":{}}`},
		{"request method alias", `{"jsonrpc":"2.0","Method":"initialize","params":{}}`},
		{"request version alias", `{"JSONRPC":"2.0","method":"initialize","params":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, request := range []struct {
				name string
				body string
			}{
				{"standalone", tc.body},
				{"batch element 1", `[{"jsonrpc":"2.0","method":"tools/call","params":{"name":"safe"}},` + tc.body + `]`},
			} {
				t.Run(request.name, func(t *testing.T) {
					var policyCalls int
					auth := &mockAuthorizer{evaluateFunc: func(context.Context, authorization.OPAInput) (*authorization.OPADecision, error) {
						policyCalls++
						return &authorization.OPADecision{Action: authorization.ActionAllow}, nil
					}}
					exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
						return server.ExchangeResult{Token: "pre-exchanged-token"}, nil
					}}
					client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
					defer cleanup()

					_, bodyResp := sendHeadersThenBody(t, client, map[string]string{":method": "POST"}, []byte(request.body))
					require.NotNil(t, bodyResp)
					immediate, ok := bodyResp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
					require.True(t, ok, "ambiguous or noncanonical MCP keys must be rejected at the gRPC boundary")
					assert.Equal(t, int32(httpv3.StatusCode_Forbidden), int32(immediate.ImmediateResponse.Status.Code))
					assert.Contains(t, string(immediate.ImmediateResponse.Body), "access_denied")
					assert.Zero(t, policyCalls, "the entire request must be validated before any policy evaluation")
				})
			}
		})
	}
}

func TestServer_OPA_MCPDecoderPreservesNestedNumericLexemes(t *testing.T) {
	body := []byte(` { "jsonrpc":"2.0", "method":"tools/call", "id":9007199254740993, "params":{"name":"deploy","arguments":{"count":9007199254740993,"nested":[1.234567890123456789,1e+3]}} } `)
	expectedArguments := map[string]any{
		"count":  json.Number("9007199254740993"),
		"nested": []any{json.Number("1.234567890123456789"), json.Number("1e+3")},
	}
	var policyBody map[string]any
	var policyCalls int
	auth := &mockAuthorizer{evaluateFunc: func(_ context.Context, input authorization.OPAInput) (*authorization.OPADecision, error) {
		policyCalls++
		mcpInput, ok := input["mcp"].(map[string]any)
		require.True(t, ok, "policy must receive the decoded MCP map")
		assert.Equal(t, json.Number("9007199254740993"), mcpInput["id"])
		assert.Equal(t, expectedArguments, mcpInput["arguments"])
		policyBody = input["parsed_body"].(map[string]any)
		assert.Equal(t, json.Number("9007199254740993"), policyBody["id"])
		assert.Equal(t, expectedArguments, policyBody["params"].(map[string]any)["arguments"])
		attributes := input["attributes"].(map[string]any)
		request := attributes["request"].(map[string]any)
		assert.Equal(t, string(body), request["http"].(map[string]any)["body"], "policy must retain the original raw request body")
		return &authorization.OPADecision{Action: authorization.ActionAllow}, nil
	}}
	exchanger := &mockExchanger{exchangeFunc: func(context.Context, string, string) (server.ExchangeResult, error) {
		return server.ExchangeResult{Token: "exchanged-token"}, nil
	}}
	client, cleanup := startTestServerWithAuthorizer(t, exchanger, auth)
	defer cleanup()

	_, bodyResp := sendHeadersThenBody(t, client, map[string]string{":method": "POST"}, body)
	streamed := bodyResp.GetRequestBody().GetResponse().GetBodyMutation().GetStreamedResponse()
	require.NotNil(t, streamed)
	assert.Equal(t, 1, policyCalls)
	var forwarded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(streamed.Body))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&forwarded))
	assert.Equal(t, policyBody, forwarded)
	assert.NotEqual(t, body, streamed.Body, "forward only the serialized values policy authorized")
	assert.True(t, streamed.EndOfStream)
}
