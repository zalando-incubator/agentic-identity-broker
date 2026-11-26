# Journey 4 policies for the exceptional credential modes. On OPA rules these policies are the only
# control on caller-asserted subject values and on audience-absent token maps.
package company.impersonation.exceptional

import rego.v1

default unsigned_subject := {"action": "deny"}

unsigned_subject := {"action": "allow"} if {
	input.context.subject_unverified
	input.context.client_assertion.sub == "gateway-1"
	input.context.subject_token.sub == "alice@example.com"
	endswith(input.context.subject_token.email, "@example.com")
}

unsigned_subject := {"action": "allow"} if {
	input.context.subject_unverified
	input.context.client_assertion.sub == "gateway-1"
	input.context.subject_token.sub in data.governance.approved_agent_subjects
}

default audienceless := {"action": "deny"}

audienceless := {"action": "allow"} if {
	input.context.client_assertion.sub == "gateway-1"
	input.context.actor_token.sub == "automation-worker"
	input.context.subject_token.sub == "alice@example.com"
}
