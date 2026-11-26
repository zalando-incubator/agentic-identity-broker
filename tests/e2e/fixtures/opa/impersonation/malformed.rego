# The decision rule exists but returns an action the impersonation contract does not accept.
package company.impersonation

import rego.v1

default result := {"action": "approval_required"}
