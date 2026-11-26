# The decision takes far longer than any evaluation timeout; evaluation stops on cancellation.
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	some i in numbers.range(1, 50000)
	some j in numbers.range(1, 50000)
	i + j < 0
}
