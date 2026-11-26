# A governance lookup through a policy-time network call, which the broker must never perform.
package company.impersonation

import rego.v1

default result := {"action": "deny"}

result := {"action": "allow"} if {
	response := http.send({"method": "GET", "url": data.lookup.url})
	response.body.approved
}
