# The decision raises a built-in error under strict built-in errors.
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	to_number(input.context.client_assertion.sub) > 0
}
