# Policies for the exceptional credential modes on OPA rules.
#
# On an OPA rule the broker performs no static check that the policy constrains caller-asserted
# values. The operator's policy is the only control on:
#   - context.subject_token of an unsigned subject (roles.subject.verification: none), including
#     email when email_expression mints it, and sub when the rule allows agent subjects;
#   - the token map of each role with audience_requirement: absent.
# Signature, issuer, lifetime, audience absence, category eligibility, cross-agent rejection,
# delegation, and scope checks stay mandatory and run whatever the policy decides.
package company.impersonation.exceptional

import rego.v1

default unsigned_subject := {"action": "deny"}

# Rule "chat-bridge" (verification: none, allowed_subject_categories: [human, agent]).
unsigned_subject := {"action": "allow"} if {
	input.context.subject_unverified
	input.context.client_assertion.sub == "chat-bridge"
	human_or_listed_agent
}

human_or_listed_agent if {
	# Human: the minted email is caller-asserted, so constrain it.
	endswith(input.context.subject_token.email, "@example.com")
	not input.context.subject_token.sub in data.governance.chat_bridge_agents
}

human_or_listed_agent if {
	# Agent: an unsigned sub equal to the target UUID is agent category on an OPA rule. Comparing
	# it with context.agent.id restricts nothing; constrain it against the agents this client may
	# represent.
	input.context.subject_token.sub in data.governance.chat_bridge_agents
}

default audienceless_client := {"action": "deny"}

# Rule "audienceless-gateway" (roles.client_assertion.audience_requirement: absent).
audienceless_client := {"action": "allow"} if {
	input.context.client_assertion.iss == "https://idp.internal.example.com"
	input.context.client_assertion.sub == "audienceless-gateway"
}
