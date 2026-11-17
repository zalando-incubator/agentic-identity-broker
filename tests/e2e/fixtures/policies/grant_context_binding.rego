package aib.extproc.authz

import rego.v1

default result := {"action": "deny", "reasons": ["unexpected permit scopes or grant update"]}

result := {"action": "allow"} if {
	c := input.context
	c.agent.canonical_id == "data-assistant"
	c.resource.canonical_id == "repo-api"
	headers := input.attributes.request.http.headers
	c.resource.scopes == split(headers["x-expected-scopes"], " ")
	expected := headers["x-expected-grant-update"]
	expected == ""
} else := {"action": "allow"} if {
	c := input.context
	c.agent.canonical_id == "data-assistant"
	c.resource.canonical_id == "repo-api"
	headers := input.attributes.request.http.headers
	c.resource.scopes == split(headers["x-expected-scopes"], " ")
	time.parse_rfc3339_ns(c.grant.updated_at) == time.parse_rfc3339_ns(headers["x-expected-grant-update"])
}
