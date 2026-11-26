# Scenario 2.1: the policy sees Envoy-compatible HTTP fields and the same context as the CEL
# activation, with JSON value types preserved. data.expected holds per-test values.
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	input.type == "impersonation"
	input.version == {"ext_authz": "v3", "encoding": "protojson"}
	envelope
	context_parity
}

envelope if {
	http := input.attributes.request.http
	http.method == "POST"
	http.path == "/oauth2/token"
	http.host == data.expected.host
	http.scheme == data.expected.scheme
	http.headers["content-type"] == "application/x-www-form-urlencoded"
	http.headers["x-tenant"] == "acme, blue"
	not http.headers.authorization
	not http.headers.cookie
	input.parsed_path == ["oauth2", "token"]
	input.parsed_query == {"trace": ["1"]}
	input.parsed_body.scope == ["read"]
	input.parsed_body.grant_type == ["urn:ietf:params:oauth:grant-type:token-exchange"]
	not input.parsed_body.client_assertion
	not input.parsed_body.actor_token
	not input.parsed_body.subject_token
	input.truncated_body == false
}

context_parity if {
	c := input.context
	c.subject_unverified == false
	c.client_assertion.sub == "gateway-1"
	c.client_assertion.tenant_level == 3
	is_number(c.client_assertion.tenant_level)
	c.actor_token.sub == "automation-worker"
	c.actor_token.groups == ["ops", "release"]
	c.subject_token.sub == "alice@example.com"
	c.subject_token.profile == {"department": "inventory", "level": 2}
	c.request == {
		"resource": "",
		"grant_type": "urn:ietf:params:oauth:grant-type:token-exchange",
		"scope": "read",
		"agent_id": data.expected.agent_id,
	}
	c.agent == {"id": data.expected.agent_id, "canonical_id": "inventory-assistant", "external_id": "inventory-agent"}
	not c.principal
	not c.grant
}
