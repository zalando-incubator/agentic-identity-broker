# The decision rule exists, so admission accepts it, but it has no default and its body is false
# for every Scenario 3.2 request: the decision is undefined at runtime.
package company.impersonation

import rego.v1

result := {"action": "allow"} if {
	input.context.client_assertion.sub == "nobody"
}
