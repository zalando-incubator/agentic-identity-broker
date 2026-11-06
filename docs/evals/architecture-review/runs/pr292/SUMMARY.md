# Behavioral probe on PR #292 (head 27f3fd9, base 2dc18c0)

These are two blind runs of `behavioral-probe-dimension.md`. Each run took about
218k tokens, 44 tool calls and 6–8 minutes. They are compared with the existing
architecture review (comment 6669614657) on the same head. None of the findings
appears in that review.

| Finding | r1 | r2 | Checked in code |
|---|---|---|---|
| Enabled-mode ExtProc consent denial returns a plain JSON 403 and drops the `WWW-Authenticate: insufficient_scope` challenge and the `-32042` URL-elicitation body that the base and spec 023 return | 🟡 | 🟡 | yes: `internal/extproc/server/delegation.go:64` |
| An agent-subject token at `POST /access/v1/evaluation` returns 500 `server_error`, which is documented as a dependency failure. ExtProc then returns 503 | 🟡 | 🟡 | yes: `evaluationStatus` in `evaluation.go:258` |
| The canonical-ID policy example `permission_sets[_].canonical_id == "repo-reader"` does not bind the permission set to the requested resource | – | 🟡 | not checked |
| A grant that expires in under 1 s gets a `max-age=0` permit, which ExtProc rejects, giving 503 | 🟢 | 🟢 | not checked |
| Agent-subject rejection lists (AGENTS.md glossary, ARCHITECTURE.md) omit evaluation | 🟢 | (in 🟡 #2) | not checked |
| `NewDelegationResolver` is used only in tests; `BuildOPAInputHeadersOnly` has no production caller | 🟢 | 🟢 | not checked |
| ADR 028 and spec 020 FR-024 header-only rules have no pointer to the ADR 047 exception | 🟢 | – | not checked |
| Broker 4xx evaluation errors (an expired token, a CEL deny) become a retryable ExtProc 503 | – | 🟢 | not checked |
