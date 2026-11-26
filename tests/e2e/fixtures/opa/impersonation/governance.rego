# Journey 1 governance policy: approved, application-assigned agents only.
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	input.type == "impersonation"
	input.attributes.request.http.method == "POST"
	input.parsed_path == ["oauth2", "token"]
	input.context.client_assertion.sub == "gateway-1"
	input.context.actor_token.sub == "automation-worker"
	registration := data.governance.agents[input.context.agent.external_id]
	registration.status == "approved"
	registration.application == "inventory-api"
}
