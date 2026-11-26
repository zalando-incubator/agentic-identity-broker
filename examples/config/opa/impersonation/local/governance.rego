# Governance policy for the impersonation rule "governed-inventory".
#
# The decision is data.company.impersonation.result. Every policy MUST define a default: a decision
# that is undefined for a request is a terminal 500 server_error, not a deny, and it also stops
# later rules from being tried.
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	input.attributes.request.http.method == "POST"
	input.parsed_path == ["oauth2", "token"]
	input.context.client_assertion.sub == "gateway-1"
	input.context.actor_token.sub == "automation-worker"

	# Governance data is keyed by the broker-resolved External ID. An Agent without one has no
	# entry, so the lookup is undefined and the default denies; there is no UUID fallback.
	registration := data.governance.agents[input.context.agent.external_id]
	registration.status == "approved"
	registration.application == "inventory-api"
}

# The equivalent CEL predicate of a rule that cannot read governance data:
#
#   client_assertion.sub == "gateway-1" && actor_token.sub == "automation-worker"
#
# OPA adds the governance lookup, which CEL cannot express.
