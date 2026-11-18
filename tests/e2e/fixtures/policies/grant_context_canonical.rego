package aib.extproc.authz

import rego.v1

# Canonical-ID delegation policy (specs/054-authzen-delegation-pip, research R18).
# It never names a deployment UUID, so equivalent deployments decide identically.

default result := {"action": "deny", "reasons": ["delegation does not cover repo-api"]}

result := {"action": "deny", "reasons": ["unexpected delegation wrapper"]} if {
	"delegation" in object.keys(input.context)
} else := {"action": "deny", "reasons": ["unexpected delegation_available flag"]} if {
	"delegation_available" in object.keys(input.context)
} else := {"action": "deny", "reasons": ["unexpected subject member"]} if {
	"subject" in object.keys(input.context)
} else := {"action": "deny", "reasons": ["permission sets unavailable"]} if {
	input.context.granted_permission_sets_available != true
} else := {"action": "deny", "reasons": ["header requested a policy denial"]} if {
	input.attributes.request.http.headers["x-policy-deny"] == "true"
} else := {"action": "allow"} if {
	c := input.context
	c.principal != ""
	c.agent.canonical_id == "data-assistant"
	c.resource.canonical_id == "repo-api"
	some ps in c.permission_sets
	ps.canonical_id == "repo-reader"
	some svc in ps.services
	svc.id == c.resource.service_id
	{id | some id in c.granted_permission_sets[ps.id]} == {s.id | some s in ps.services}
}
