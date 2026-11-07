package aib.extproc.authz

import rego.v1

# Allows only when disabled-mode input carries none of the delegation members
# (specs/054-authzen-delegation-pip US4.1).
default result := {"action": "allow"}

result := {"action": "deny", "reasons": ["delegation member present in disabled mode"]} if {
	some member in ["principal", "agent", "resource", "grant", "permission_sets", "delegation", "delegation_available"]
	member in object.keys(input.context)
}
