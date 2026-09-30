package server

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
)

func FuzzMCPApprovalMatchesOPA(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","method":"tools/call","id":7,"params":{"name":"deploy","arguments":{"path":"/prod"}}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dangerous","arguments":{"path":"/prod"},"Name":"safe","Arguments":{"path":"/tmp"}}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list","arguments":{"path":"/tmp","path":"/prod"}}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list","arguments":"not an object"}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list"}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list","arguments":null}}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		input, err := authorization.BuildOPAInput("mcp", body, nil, "", authorization.ContextInput{})
		if err != nil || input["type"] != "mcp_tool_call" {
			return
		}
		mcpInput := input["mcp"].(*authorization.MCPInput)
		invocation, _, err := approvalInvocation(input, &requestState{}, &authorization.OPADecision{Action: authorization.ActionApprovalRequired})
		if err != nil {
			t.Fatalf("OPA accepted tool call but approval gate rejected it: %v", err)
		}
		if invocation.ToolName != mcpInput.ToolName || !reflect.DeepEqual(invocation.Arguments, mcpInput.Arguments) {
			t.Fatalf("OPA saw %q %v; approval gate saw %q %v", mcpInput.ToolName, mcpInput.Arguments, invocation.ToolName, invocation.Arguments)
		}

		response := canonicalMCPBody(input["parsed_body"], &extprocv3.HttpBody{Body: body, EndOfStream: true}, 0)
		request := response.GetRequestBody()
		if request == nil || request.GetResponse().GetBodyMutation().GetStreamedResponse() == nil {
			t.Fatal("authorized message was not forwarded")
		}
		var upstream map[string]any
		decoder := json.NewDecoder(bytes.NewReader(request.GetResponse().GetBodyMutation().GetStreamedResponse().Body))
		decoder.UseNumber()
		if err := decoder.Decode(&upstream); err != nil {
			t.Fatalf("upstream message is not JSON: %v", err)
		}
		if !reflect.DeepEqual(upstream, input["parsed_body"]) {
			t.Fatalf("upstream view differs from OPA view: %v != %v", upstream, input["parsed_body"])
		}
		params := upstream["params"].(map[string]any)
		args, present := params["arguments"]
		if upstream["method"] != "tools/call" || params["name"] != invocation.ToolName || (present && !reflect.DeepEqual(args, invocation.Arguments)) || (!present && invocation.Arguments != nil) {
			t.Fatalf("upstream invocation differs from approval: %v vs %v", params, invocation)
		}
	})
}
