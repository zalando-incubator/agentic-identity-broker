package aib.extproc.authz

import rego.v1

# Allows only when disabled-mode input carries the legacy context and none of the
# delegation members (specs/054-authzen-delegation-pip US4.1).
default result := {"action": "deny", "reasons": ["disabled-mode context missing or carries delegation members"]}

delegation_members := ["principal", "agent", "resource", "grant", "permission_sets", "delegation", "delegation_available"]

result := {"action": "allow"} if {
	is_object(input.context)
	is_boolean(input.context.granted_permission_sets_available)
	every member in delegation_members {
		not member in object.keys(input.context)
	}
}
