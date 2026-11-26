# Scenario 2.2: the policy requires a verified client that the request does not present. Caller
# headers cannot supply it, because context is built only from validated credentials.
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	input.context.client_assertion.sub == data.required_client
}
