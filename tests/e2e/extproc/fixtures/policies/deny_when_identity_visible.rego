package aib.extproc.authz

import rego.v1

# Spec 055 US3: denies whenever the input carries an identity, so an allowed
# request proves ExtProc omitted both fields.

default result := {"action": "allow"}

identity_visible if input.context.principal

identity_visible if input.context.agent_id

result := {"action": "deny", "reasons": ["identity is visible"]} if identity_visible
