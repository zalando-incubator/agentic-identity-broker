# Quickstart & Validation Guide: OAuth2 User Impersonation

**Feature**: 20260825-120000-oauth2-user-impersonation
**Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md) | **Data model**: [data-model.md](data-model.md)

This guide proves the impersonation feature end-to-end. It is a validation/run guide only —
implementation lives in the code and E2E suites produced during implementation. Config details are
in [contracts/impersonation-config.yaml](contracts/impersonation-config.yaml); API details in
[contracts/openapi-token-impersonation.yaml](contracts/openapi-token-impersonation.yaml).

---

## Prerequisites

- Broker built: `just build` (binary at `./bin/agentic-identity-broker`).
- Broker in **local** mode with signing keys and an `encryption` block configured (impersonation
  reuses the local issuer's signing key).
- An external IdP (or a test JWKS server, as used by `internal/adapters/jwks/adapter_test.go`) that
  can sign client-assertion / actor / subject JWTs with an approved asymmetric algorithm.
- The `oauth2_authorization_server.impersonation` block from the config contract.
- The impersonated subject must already hold an active `UserGrant` for the registered target agent. The subject identity extracted from `subject_token` is the user-grant principal.

## Setup

The impersonation config contract is an **overlay/snippet** (only the
`oauth2_authorization_server` subtree). The broker's top-level `Config` also requires `log`,
`server`, and `storage`, plus an `encryption` block for local signing keys
(`internal/ports/config.go:38-45`), so it is NOT bootable on its own. Build a complete config by
merging the impersonation subtree into a local-mode base:

```bash
# Start from a complete local-mode example (has log/server/storage), then add the impersonation
# subtree from contracts/impersonation-config.yaml under oauth2_authorization_server.
cp examples/config/oauth2-server-mode.yaml /tmp/impersonation-config.yaml
# Ensure mode: local, an encryption block, and paste the `impersonation:` block from
# specs/20260825-120000-oauth2-user-impersonation/contracts/impersonation-config.yaml
# (During implementation this complete file ships as examples/config/impersonation.yaml — FR-014.)

export IDENTITY_BROKER_CONFIG_PATH=/tmp/impersonation-config.yaml
just build && ./bin/agentic-identity-broker serve
```

Startup MUST fail closed with an actionable, field-indexed error if any of CR-001..CR-008 is
violated (e.g. remove `allowed_algorithms` from a trusted issuer, or add the block in proxy mode,
or use an `HS256` algorithm, or configure an unverified rule whose predicate omits
`subject_token`).

Impersonation additionally fails closed at startup if user-delegation verification or the end-user public URL used for the consent `error_uri` is unavailable (CR-010).

## Run the validation suites

```bash
# Functional backend E2E acceptance suite (excludes SC-001):
ginkgo -v --label-filter="!performance" --focus "OAuth2 User Impersonation" ./tests/e2e/

# Dedicated warmed local-mode, signed-subject SC-001 measurement (100 requests):
just test-e2e-performance

# Domain/unit + config validation:
just test

# Full gate before handoff:
just verify
```

---

## Scenario checklist (maps 1:1 to spec acceptance scenarios)

Each row is one `It()` in `tests/e2e/impersonation_test.go`. Expected result column is the
observable proof.

### User Story 1 — Privileged client mints an impersonated token

| # | Action | Expected |
|---|---|---|
| 1 | Valid request; `audience` = `$IMPERSONATION_AUDIENCE_PREFIX/$TARGET_AGENT_ID`; JWT actor/subject types; jwt-bearer assertion type; active subject delegation | 200; token `sub` = subject, `agent_id` and CEL agent fields = target, `act.iss` = validated actor-token issuer, `act.sub` = actor |
| 2 | `subject_token_type`/`actor_token_type` absent, malformed, or unsupported | 400 `invalid_request`; no token |
| 3 | Inspect issued token | issuer/lifetime/signing/base claims and `aud` follow local policy; target fields are target-derived; JWT `scope` is the granted request scope and the response omits `scope` only when none was granted |
| 4 | Different, absent, or multiple `audience` | impersonation does NOT activate; pre-existing routing applies |
| 5 | Proxy or hybrid mode targeting a valid suffixed audience | rejected; no impersonated token or upstream forwarding |
| 6 | Unverified subject with a valid registered target, active subject delegation, and binding rule | 200; target-derived `agent_id`, subject/email local-policy claims, validated actor-token `act.iss`, and `act.sub` |

### User Story 2 — Operator defines trusted sources as rules

| # | Action | Expected |
|---|---|---|
| 1 | Configure client-assertion issuer; send valid client assertion from it | validated; privileged-client identity derived |
| 2 | Configure actor + signed-subject issuers; send valid tokens | non-empty actor + subject identities via the rule's extraction rules |
| 3 | Credential from a valid issuer but no rule trusts that issuer for that role and matches | rejected |
| 4 | Rule omits a required issuer/extraction/audience/authorization at startup | startup fails; error names the offending rule |
| 5 | Two rules validate the client assertion; only the second's predicate permits the actor/subject | falls through rule 1, applies rule 2; shared issuer permitted |

### User Story 3 — Broker rejects unsafe/unauthorized impersonation

| # | Action | Expected |
|---|---|---|
| 1 | Subject under RFC 8693 JWT type with non-verifiable signature (even if operator tries to allow it) | rejected; no token (distinct from unverified mode) |
| 2 | Any client assertion/actor/signed subject fails signature/issuer/audience/expiry/nbf | rejected; no token |
| 3 | Valid client assertion no predicate permits | 403 `access_denied`; no token |
| 4 | Token lacks claim/structure to extract actor or subject identity | OAuth2 error; no partial token |
| 5 | Unverified request; no rule accepts unverified mode for this client/actor, OR predicate does not permit subject; OR predicate ignores `subject_token` | rejected; no token |

### User Story 4 — Operator audits impersonation decisions

| # | Action | Expected |
|---|---|---|
| 1 | Successful request | audit records privileged client, target agent, actor, subject, selected rule, issuer ids/roles, routing prefix, outcome, correlation |
| 2 | Failure before all identities established | audit event records failure category, rule evaluation outcome, only safely-available identities |
| 3 | Inspect any impersonation audit event | contains NO client assertion, actor/subject/access token, signing key, or credential value |

### User Story 5 — Impersonation honors user delegation

| # | Action | Expected |
|---|---|---|
| 1 | Valid signed-subject request with an active delegation for subject + target agent | 200; impersonated token |
| 2 | Otherwise valid request but no subject delegation | 403 `access_denied`; no token; `error_uri` = target-agent consent page |
| 3 | Otherwise valid request but expired subject delegation | Same 403 body as missing delegation; audit records expired category |
| 4 | Unverified subject with no delegation | 403 `access_denied`; no token |
| 5 | Delegation verifier storage failure | 500 `server_error`; no token and no `error_uri` |

### Edge cases (additional `It()` blocks)

- `scope=read` for a target allowing `read` → 200 with `scope: "read"` in the JWT and response; an unlisted scope → 400 `invalid_scope` with no token; an empty target allow-list is unrestricted. `resource` → 400 `invalid_request`.
- A bare suffix or a suffix matching neither identifier form → 400 `invalid_request`; an unknown target of either form → 400 `invalid_target`.
- `requested_token_type` present but ≠ access-token type → 400 `invalid_request`.
- actor identity equals subject identity → permitted; `act.sub == sub`.
- signing-key metadata for an issuer unavailable → rules depending on it fall through; if none match, fail closed.
- Missing or expired subject delegation → 403 `access_denied` with `<end-user-public-url>/consent/agent/<target-agent-id>` as `error_uri`; it is terminal and applies to unverified subjects. A delegation lookup failure → 500 `server_error` with no `error_uri`.

---

## Manual smoke test (single signed-subject request)

```bash
curl -s -X POST http://localhost:8000/oauth2/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=urn:ietf:params:oauth:grant-type:token-exchange' \
  --data-urlencode "audience=$IMPERSONATION_AUDIENCE_PREFIX/$TARGET_AGENT_ID" \
  --data-urlencode 'client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer' \
  --data-urlencode "client_assertion=$CLIENT_ASSERTION_JWT" \
  --data-urlencode 'actor_token_type=urn:ietf:params:oauth:token-type:jwt' \
  --data-urlencode "actor_token=$ACTOR_JWT" \
  --data-urlencode 'subject_token_type=urn:ietf:params:oauth:token-type:jwt' \
  --data-urlencode "subject_token=$SUBJECT_JWT" \
  --data-urlencode 'scope=read' | jq .
# Decode access_token: sub == subject; agent_id and policy agent fields equal target; act.iss == validated actor-token issuer; act.sub == actor;
# aud follows local token_claims_expression; scope == "read" when the target allows it.
# Before this request, create an active UserGrant for the extracted subject and $TARGET_AGENT_ID.
# Without that delegation, expect 403 access_denied with error_uri=$ENDUSER_PUBLIC_URL/consent/agent/$TARGET_AGENT_ID.
```

Use `audience=$IMPERSONATION_AUDIENCE_PREFIX/$TARGET_AGENT_CANONICAL_ID` to address the same target by canonical ID.

## Done criteria

- All scenario rows pass (green) in `tests/e2e/impersonation_test.go`.
- `just verify` passes.
- ADR 20260825-120000 is **Accepted** in the base branch (governance PR merged) before this feature merges.
- ADR 20260825-120100 is **Accepted**.
