# Checks that `weaver registry live-check` adds to its own for `just telemetry-live-check`.
# Weaver itself reports a metric or an attribute that the registry does not list, and a
# span kind, a metric instrument or a unit that differs from the registry.
package live_check_advice

import rego.v1

# A span that the registry does not list.
deny contains finding("span_not_in_registry", message) if {
	input.sample.span
	object.get(input, "registry_group", null) == null
	message := sprintf("Span '%s' is not in the registry.", [input.sample.span.name])
}

# A span or a metric from another instrumentation scope than the registry names.
deny contains finding("scope_mismatch", message) if {
	some kind in ["span", "metric"]
	sample := input.sample[kind]
	want := input.registry_group.annotations.scope
	got := input.instrumentation_scope.name
	want != got
	message := sprintf("%s '%s' has scope '%s', and the registry has '%s'.", [kind, sample.name, got, want])
}

# A span with status Error, when the registry says that the span does not set it.
deny contains finding("unexpected_error_status", message) if {
	input.sample.span.status.code == "error"
	input.registry_group.annotations.error_status == false
	message := sprintf("Span '%s' has status Error, and the registry has error_status: false.", [input.sample.span.name])
}

# A value that the span or the metric does not list. The `examples` of a reference to
# an enumerated attribute are the values that this span or metric sets.
deny contains finding("value_not_listed", message) if {
	attribute := input.sample.attribute
	input.registry_attribute.type.members
	listed := input.registry_attribute.examples
	not attribute.value in listed
	message := sprintf("Attribute '%s' has value '%v', and the registry lists %v here.", [attribute.name, attribute.value, listed])
}

finding(id, message) := {
	"type": "advice",
	"advice_type": id,
	"advice_level": "violation",
	"advice_context": {},
	"message": message,
}
