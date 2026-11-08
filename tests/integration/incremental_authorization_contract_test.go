package integration_test

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func incrementalAuthorizationContract(t *testing.T) (map[string]any, *jsonschema.Compiler) {
	t.Helper()
	encoded, err := os.ReadFile("../../api/enduser/openapi.yaml")
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(encoded, &document))
	root := credentialSchemaValue(t, document).(map[string]any)
	credentialOpenAPINullable(root)
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("https://broker.test/enduser-contract.json", root))
	return document, compiler
}

func incrementalAuthorizationSchema(t *testing.T, compiler *jsonschema.Compiler, name string) *jsonschema.Schema {
	t.Helper()
	schema, err := compiler.Compile("https://broker.test/enduser-contract.json#/components/schemas/" + name)
	require.NoError(t, err)
	return schema
}

func TestIncrementalAuthorizationRevisionContract(t *testing.T) {
	document, compiler := incrementalAuthorizationContract(t)
	assert.Equal(t, "1.3.0", document["info"].(map[string]any)["version"])
	paths := document["paths"].(map[string]any)
	authorize := paths["/oauth2/authorize"].(map[string]any)["get"].(map[string]any)
	description := authorize["description"].(string)
	for _, text := range []string{"302 access_denied", "authorization session is invalid or expired", "302 invalid_scope", "state is omitted if absent", "Redirect validation precedes continuation validation"} {
		assert.Contains(t, description, text)
	}
	token := paths["/oauth2/token"].(map[string]any)["post"].(map[string]any)
	for _, text := range []string{"Before forwarding", "400 invalid_scope", "requested service scope is not permitted", "500 server_error", "no upstream call", "No provider-session usability or grant-coverage check"} {
		assert.Contains(t, token["description"], text)
	}
	schema := incrementalAuthorizationSchema(t, compiler, "OAuth2Error")
	for _, status := range []string{"400", "500"} {
		examples := token["responses"].(map[string]any)[status].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["examples"].(map[string]any)
		name := "proxy_service_scope_rejected"
		if status == "500" {
			name = "proxy_service_scope_lookup_failed"
		}
		example, ok := examples[name]
		if assert.True(t, ok, "missing %s", name) {
			assert.NoError(t, schema.Validate(credentialSchemaValue(t, example.(map[string]any)["value"])))
		}
	}
	encoded, err := os.ReadFile("../../specs/055-incremental-service-authorization/spec.md")
	require.NoError(t, err)
	example := regexp.MustCompile(`(?m)^\{"data":\{"service":.*$`).Find(encoded)
	require.NotEmpty(t, example)
	var body any
	require.NoError(t, json.Unmarshal(example, &body))
	assert.NoError(t, incrementalAuthorizationSchema(t, compiler, "ServiceSessionDetailResponse").Validate(body))
}

func TestIncrementalAuthorizationSessionDetailContract(t *testing.T) {
	document, compiler := incrementalAuthorizationContract(t)
	schema := incrementalAuthorizationSchema(t, compiler, "ServiceSessionDetailResponse")
	paths := document["paths"].(map[string]any)
	operation := paths["/api/third-party/sessions/{service}"].(map[string]any)["get"].(map[string]any)
	assert.Equal(t, "getServiceSessionDetail", operation["operationId"])
	assert.Equal(t, []any{map[string]any{"preAuth": []any{}}}, operation["security"])
	responses := operation["responses"].(map[string]any)
	for _, status := range []string{"200", "401", "404", "500"} {
		assert.Contains(t, responses, status)
	}
	examples := responses["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["examples"].(map[string]any)
	for name, example := range examples {
		t.Run(name, func(t *testing.T) {
			body := credentialSchemaValue(t, example.(map[string]any)["value"])
			assert.NoError(t, schema.Validate(body))
			data := body.(map[string]any)["data"].(map[string]any)
			for _, field := range []string{"service", "session", "dependent_agents"} {
				t.Run("requires_"+field, func(t *testing.T) {
					copy := credentialSchemaValue(t, body).(map[string]any)
					delete(copy["data"].(map[string]any), field)
					assert.Error(t, schema.Validate(copy))
				})
			}
			if data["session"] != nil {
				delete(data["session"].(map[string]any), "id")
				assert.Error(t, schema.Validate(body), "non-null sessions retain SessionStatus requirements")
			}
		})
	}
}

func TestIncrementalAuthorizationConsentContract(t *testing.T) {
	_, compiler := incrementalAuthorizationContract(t)
	schema := incrementalAuthorizationSchema(t, compiler, "RequestedServiceCoverage")
	for _, usability := range []string{"usable", "missing", "unusable", "unavailable"} {
		t.Run(usability, func(t *testing.T) {
			body := map[string]any{
				"service_id":      "880e8400-e29b-41d4-a716-446655440010",
				"required_scopes": []any{"repo", "user:email"},
				"usability":       usability,
				"covering_permission_sets": []any{map[string]any{
					"permission_set_id":        "770e8400-e29b-41d4-a716-446655440001",
					"supplied_required_scopes": []any{"repo"},
					"usability":                usability,
				}},
			}
			assert.NoError(t, schema.Validate(body))
			candidate := body["covering_permission_sets"].([]any)[0].(map[string]any)
			candidate["usability"] = "connected"
			assert.Error(t, schema.Validate(body), "connectionStatus is not contribution usability")
			candidate["usability"] = usability
			body["usability"] = "unusable_definitive"
			assert.Error(t, schema.Validate(body), "internal classifications are not public API values")
		})
	}
	document, _ := incrementalAuthorizationContract(t)
	components := document["components"].(map[string]any)["schemas"].(map[string]any)
	detail := components["GetAgentDetailResponse"].(map[string]any)["properties"].(map[string]any)["data"].(map[string]any)
	assert.Contains(t, detail["required"], "requested_service_ids")
	assert.Contains(t, detail["required"], "requested_service_coverage")
	detailSchema := incrementalAuthorizationSchema(t, compiler, "GetAgentDetailResponse")
	operation := document["paths"].(map[string]any)["/api/consent/agents/{agent-id}"].(map[string]any)["get"].(map[string]any)
	examples := operation["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["examples"].(map[string]any)
	for name, example := range examples {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, detailSchema.Validate(credentialSchemaValue(t, example.(map[string]any)["value"])))
		})
	}
}

func TestIncrementalAuthorizationExchangeErrorContract(t *testing.T) {
	document, compiler := incrementalAuthorizationContract(t)
	tokenSchema := incrementalAuthorizationSchema(t, compiler, "OAuth2TokenResponse")
	operation := document["paths"].(map[string]any)["/oauth2/token"].(map[string]any)["post"].(map[string]any)
	examples := operation["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["examples"].(map[string]any)
	for _, name := range []string{"local_service_scope", "narrowed_local_refresh"} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, tokenSchema.Validate(credentialSchemaValue(t, examples[name].(map[string]any)["value"])))
		})
	}
	schema := incrementalAuthorizationSchema(t, compiler, "OAuth2Error")
	for _, code := range []string{"invalid_grant", "access_denied", "invalid_scope", "server_error"} {
		t.Run(code, func(t *testing.T) {
			assert.NoError(t, schema.Validate(map[string]any{"error": code}))
		})
	}
	assert.NoError(t, schema.Validate(map[string]any{
		"error":     "invalid_grant",
		"error_uri": "https://broker.example.com/api/third-party/880e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions%2Fgithub",
		"scope":     "https://broker.example.com/sessions/github",
	}))
}
