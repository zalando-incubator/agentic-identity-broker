package opa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func compileSources(t *testing.T, sources map[string]string) (*candidateCompilation, *rejection) {
	t.Helper()
	modules, rej := parseModules(sources)
	require.Nil(t, rej)
	return compileCandidate(modules)
}

func TestRestrictedCapabilitiesRejectEveryRemovedBuiltin(t *testing.T) {
	calls := map[string]string{
		"http.send":              `http.send({"method": "GET", "url": "http://127.0.0.1:1"})`,
		"net.lookup_ip_addr":     `net.lookup_ip_addr("example.com")`,
		"opa.runtime":            `opa.runtime()`,
		"rand.intn":              `rand.intn("seed", 10)`,
		"uuid.rfc4122":           `uuid.rfc4122("seed")`,
		"io.jwt.encode_sign":     `io.jwt.encode_sign({"alg": "HS256"}, {}, {"kty": "oct", "k": "AA"})`,
		"io.jwt.encode_sign_raw": `io.jwt.encode_sign_raw("{}", "{}", "{}")`,
		"json.verify_schema":     `json.verify_schema({})`,
		"json.match_schema":      `json.match_schema({}, {})`,
	}
	require.Len(t, calls, 9)
	for builtin, call := range calls {
		t.Run(builtin, func(t *testing.T) {
			_, rej := compileSources(t, map[string]string{
				"/decision.rego":  "package aib.broker.impersonation\n\nimport rego.v1\n\ndefault result := {\"action\": \"deny\"}\n",
				"/unreached.rego": "package helpers\n\nimport rego.v1\n\nunused := " + call + "\n",
			})
			require.NotNil(t, rej, "a call in an unreachable module still rejects the candidate")
			assert.Equal(t, reasonProhibitedBuiltin, rej.reason)
			assert.Equal(t, builtin, rej.builtin)
		})
	}
}

func TestRestrictedCapabilitiesKeepClockBuiltins(t *testing.T) {
	compiled, rej := compileSources(t, map[string]string{
		"/decision.rego": "package aib.broker.impersonation\n\nimport rego.v1\n\n" +
			"now := time.now_ns()\n\nverified := io.jwt.decode_verify(\"a.b.c\", {\"secret\": \"x\"})\n\n" +
			"default result := {\"action\": \"deny\"}\n\nresult := {\"action\": \"allow\"} if {\n\tprint(\"never emitted\")\n\tnow > 0\n}\n",
	})
	require.Nil(t, rej)
	require.NotNil(t, compiled)
}

func TestCompileFailuresOtherThanRemovedBuiltinsAreCompileFailed(t *testing.T) {
	for name, source := range map[string]string{
		"unknown function": "package p\n\nimport rego.v1\n\nx := not_a_builtin(1)\n",
		"type error":       "package p\n\nimport rego.v1\n\nx := 1 + \"a\"\n",
		"recursion":        "package p\n\nimport rego.v1\n\na if b\n\nb if a\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, rej := compileSources(t, map[string]string{"/p.rego": source})
			require.NotNil(t, rej)
			assert.Equal(t, reasonCompileFailed, rej.reason)
			assert.Empty(t, rej.builtin)
		})
	}
	_, rej := parseModules(map[string]string{"/p.rego": "package p\n\nx := {"})
	require.NotNil(t, rej)
	assert.Equal(t, reasonCompileFailed, rej.reason)
}

func TestDecisionPresenceUsesExactRuleLookup(t *testing.T) {
	compiled, rej := compileSources(t, map[string]string{
		"/defined.rego": "package company.impersonation\n\nimport rego.v1\n\ndefault result := {\"action\": \"deny\"}\n",
		"/nested.rego":  "package company.nested.result\n\nimport rego.v1\n\nallow := true\n",
	})
	require.Nil(t, rej)
	present := compiled.decisionPresence([]ports.ImpersonationPolicyDecisionRef{
		{Rule: "defined", Package: "company.impersonation", Decision: "result"},
		{Rule: "nested-only", Package: "company.nested", Decision: "result"},
		{Rule: "data-only", Package: "governance", Decision: "agents"},
		{Rule: "missing", Package: "aib.broker.impersonation", Decision: "result"},
	})
	assert.Equal(t, map[string]bool{"defined": true, "nested-only": false, "data-only": false, "missing": false}, present)
}
