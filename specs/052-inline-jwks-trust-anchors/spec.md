# Feature Specification: Inline and File-Based JWKS for Trust Anchors

**Feature Branch**: `052-inline-jwks-trust-anchors`

**Created**: 2026-10-12

**Status**: Draft

**Input**: User description: "https://github.com/zalando-incubator/agentic-identity-broker/issues/185" — *Support inline JWKS in token exchange and other places*

## Context

Every operator-configured trust anchor that verifies signed credentials currently gets its public verification keys over the network. The key set comes from an explicit `jwks_uri` or from discovery through the issuer's metadata. This applies to three trust anchors:

- The token-exchange client-assertion trust anchor (ADR 029). Machine-facing approval authentication reuses it.
- Each trusted issuer of an impersonation rule ([OAuth2 User Impersonation](../037-oauth2-user-impersonation/spec.md)).
- JWT pre-authentication on the end-user server and on the admin server ([JWT Pre-Authentication](../016-jwt-preauth/spec.md)).

Issue #185 says this assumption fails when infrastructure holds the signing key locally. An agent gateway, such as agentgateway, can sign privileged client assertions with its own key. Then no web endpoint publishes the matching public key. To satisfy the broker, the operator must run a key-set endpoint that serves only a static document. The issue discussion states the principle: the broker needs a trusted public key, not a website that serves one. The discussion settled on two alternatives as sufficient for now:

1. An inline JWK Set document in the broker configuration.
2. A configured path to a file that contains the JWK Set document.

This feature changes only **where** a trust anchor's public keys come from. It does not change which issuer is trusted. It does not change how credentials are validated. It does not change which credentials are accepted. It extends ADR 029's trust-anchor configuration. It does not supersede it.

## Clarifications

### Session 2026-10-12

- Q: How does the broker handle changes to a key-set file after startup? → A: It re-reads the file within the trust anchor's refresh bounds. When a re-read fails, the broker keeps the last valid key set and reports degraded health. This matches the behaviour of remote key sets, so a key rotation that updates a mounted file takes effect without a restart.
- Q: Where can a `jwks_file` path point? It could otherwise point at any file in the broker's runtime environment that the process can read. → A: Only to a file inside one operator-configured trust directory (`security.trust_material_dir`). The broker checks containment after it follows symbolic links. When no trust directory is configured, `jwks_file` is rejected. These additional hardening controls were considered and not adopted:
  - rejecting writable files;
  - stricter non-disclosing errors and refusal of pseudo-filesystems;
  - audit-log content digests;
  - deployment-chart-only mounts.

### Session 2026-10-16

- Q: Is ADR 040 accepted as drafted? → A: Yes. The stakeholder accepted it in this PR. It was then revised for the admin-server decision below.
- Q: Are absolute symbolic links inside the trust directory rejected (research R4)? → A: Yes. Only relative links are followed, as FR-023 states.
- Q: Should `jwks` and `jwks_file` on `server.admin.authentication.jwt` be rejected or supported (research R9)? → A: Supported. The admin server's JWT pre-authentication is a separate trust anchor with the same rules as the end-user server (FR-002, User Story 5 scenario 4).
- Q: Are the `/health` component contract and the five example journeys confirmed? → A: Yes, both, as written.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Trust a locally signing gateway for token exchange with an inline key set (Priority: P1)

An operator runs a gateway that signs RFC 8693 client assertions with a key that it holds locally. The operator configures the token-exchange client-assertion trust anchor with the gateway's issuer identifier and an inline JWK Set that contains the gateway's public key. The broker starts without contacting an issuer metadata or key-set endpoint for that trust anchor. It accepts token-exchange and machine-facing approval requests whose client assertions are signed with that key.

**Why this priority**: The issue asks for this case. Without it, a locally signing gateway needs an extra network service whose only job is to publish a static public key.

**Independent Test**: Start the broker in local mode. Configure an inline client-assertion key set, and make no issuer metadata or key-set endpoint reachable for that issuer. Perform a token exchange with a client assertion that the matching private key signed, and confirm that the exchange succeeds. Repeat with an assertion that a different key signed, and confirm that the broker rejects it.

**Acceptance Scenarios**:

1. **Given** local mode with a client-assertion trust anchor that declares an issuer identifier and an inline JWK Set, and no reachable metadata or key-set endpoint for that issuer, **When** the broker starts, **Then** startup succeeds and the broker sends no outbound request to get that trust anchor's keys.
2. **Given** that configuration, **When** a gateway submits a token exchange with a client assertion that is otherwise valid and signed by a key in the inline set, **Then** the exchange succeeds with the same outcome as when the key comes from a remote key set.
3. **Given** that configuration, **When** a gateway submits a client assertion signed by a key that is not in the inline set, **Then** the broker rejects the request with the same error that an unknown remote key produces, and issues no token.
4. **Given** that configuration, **When** a gateway authenticates to a machine-facing approval endpoint with a client assertion signed by a key in the inline set, **Then** the broker authenticates the gateway as it does for a remotely sourced key.
5. **Given** proxy or hybrid mode with no explicit client-assertion issuer, **When** the operator configures an inline client-assertion key set, **Then** the expected client-assertion issuer stays the upstream issuer, client assertions verify only against the inline keys, and subject tokens still verify against the upstream issuer's keys.

---

### User Story 2 - Provide the key set as a file (Priority: P1)

An operator does not embed the JWK Set in the broker configuration. The operator keeps it in a file that the deployment platform manages, for example a file mounted from a cluster secret or configuration object, and references the file by path.

**Why this priority**: The issue names the file source as the second required alternative. It keeps large key documents out of the main configuration, and the operator manages it with the same platform tools as other mounted material.

**Independent Test**: Configure a trust directory, and configure the client-assertion trust anchor with the path of a readable file inside it that contains a valid JWK Set. Confirm that the broker starts and verifies client assertions with the file's keys. Then configure a missing path, and separately a path outside the trust directory. Confirm that startup fails for each.

**Acceptance Scenarios**:

1. **Given** a configured trust directory and a client-assertion trust anchor that references a readable file inside it with a valid JWK Set, **When** the broker starts, **Then** startup succeeds and client assertions signed by a key in the file verify successfully.
2. **Given** a trust anchor that references a file path that does not exist, is not readable, or is not a regular file, **When** the broker starts, **Then** startup fails with an error that names the configuration setting and the path.
3. **Given** a running broker with a file-based key set, **When** an operator replaces the file's contents with a different valid key set, **Then** the broker verifies with the new key set within the trust anchor's refresh bounds without a restart, and rejects credentials signed only by keys that the replacement removed.
4. **Given** a running broker with a file-based key set, **When** the file becomes unreadable or its new contents fail validation, **Then** the broker keeps verifying with the last valid key set until the trust anchor's maximum refresh interval has passed since the last successful read (FR-011), reports degraded key-set health, and logs the failure without file contents. **When** a valid file returns, **Then** the broker applies it and reports healthy again.
5. **Given** the supported deployment package, **When** an operator references an existing cluster secret or configuration object that contains a JWK Set, **Then** the broker receives the key set as a file at the configured path without a custom image.
6. **Given** a configured trust directory, **When** a trust anchor references a key-set file outside it, **Then** startup fails with an error that names the setting, the path, and the trust directory, and the broker does not read the file. Examples of such files are the broker's configuration file and a platform-provided credential file.
7. **Given** a configured trust directory, **When** a key-set path inside it leads, through a symbolic link or `..` segments, to a file outside the directory, **Then** startup fails as in scenario 6.
8. **Given** no configured trust directory, **When** a trust anchor declares a key-set file, **Then** startup fails with an error that names the setting and the missing trust directory.
9. **Given** a trust directory where a cluster platform has mounted a secret or configuration object through its usual chain of symbolic links inside that directory, **When** a trust anchor references a file in the mount, **Then** startup succeeds.

---

### User Story 3 - Fail closed on invalid or ambiguous key configuration (Priority: P1)

An operator makes a mistake in a local key set. For example, the operator configures two key sources for one trust anchor, pastes malformed JSON, or pastes a private key instead of a public key. The broker refuses to start and explains which setting is wrong. It never echoes key material.

**Why this priority**: Security-first (Constitution Principle I). A local key source is a trust decision. A malformed or ambiguous key source must never become a permissive or partially configured trust anchor.

**Independent Test**: Start the broker with each invalid configuration in the scenarios that follow. Confirm that each one prevents startup with an error that names the offending setting and contains no key material.

**Acceptance Scenarios**:

1. **Given** a trust anchor that declares more than one of a key-set URI, an inline key set, and a key-set file, **When** the broker starts, **Then** startup fails and identifies the trust anchor and the conflicting settings.
2. **Given** an inline or file key set that is not a valid JWK Set document, contains no keys, or exceeds the size bound, **When** the broker starts, **Then** startup fails and identifies the trust anchor.
3. **Given** an inline or file key set that contains private key material, **When** the broker starts, **Then** startup fails, and the error and logs do not contain any part of the key material.
4. **Given** an inline or file key set that contains a symmetric key, or a key that declares an algorithm outside the broker-approved asymmetric set, **When** the broker starts, **Then** startup fails and identifies the trust anchor.
5. **Given** an inline or file key set that contains two keys with the same key identifier, **When** the broker starts, **Then** startup fails and identifies the duplicate identifier.
6. **Given** an inline key set and explicit refresh bounds on the same trust anchor, **When** the broker starts, **Then** startup fails because an inline key set never refreshes.
7. **Given** local or hybrid mode with a client-assertion issuer that equals the broker's own issuer, **When** the trust anchor uses an inline or file key set, **Then** startup still fails, as it does for remote key sets.
8. **Given** a valid inline or file key set, **When** the broker starts, **Then** it logs each trust anchor's key source kind, key count, and key identifiers, and does not log key material.

---

### User Story 4 - Use local key sets for impersonation trusted issuers (Priority: P2)

An operator who defines OAuth2 user-impersonation rules trusts a gateway that signs client assertions and actor tokens with a locally held key. The operator declares the gateway's public keys inline or by file on the rule's trusted issuer. The operator does not run a key-set endpoint.

**Why this priority**: Impersonation trusted issuers act as client-assertion trust anchors. The issue's "other places" covers them. They are secondary because impersonation is an optional, local-mode-only capability.

**Independent Test**: Configure an impersonation rule whose trusted issuer declares an inline key set. Submit an impersonation request whose client assertion and actor token were signed by the matching private key. Confirm that the broker issues a token. Repeat with a key that the issuer does not declare, and confirm that the broker rejects the request.

**Acceptance Scenarios**:

1. **Given** an impersonation rule whose trusted issuer declares an inline key set, **When** the broker starts, **Then** startup succeeds without issuer metadata discovery, and impersonation requests whose credentials those keys signed are validated as they are today.
2. **Given** an impersonation rule whose trusted issuer declares a key-set file, **When** the broker starts, **Then** credentials signed by the file's keys are validated as they are today.
3. **Given** a trusted issuer with a local key set that permits only `ES256`, **When** a credential signed by one of its keys uses a different algorithm, **Then** the broker rejects the credential.
4. **Given** two trusted issuers with the same issuer identifier in different rules that declare different key sources, **When** each rule evaluates a credential, **Then** each rule verifies only against the keys that its own trusted issuer declares.

---

### User Story 5 - Use local key sets for JWT pre-authentication (Priority: P3)

An operator runs the broker behind infrastructure such as a reverse proxy or service mesh. That infrastructure issues signed identity JWTs with a locally held key. The operator declares the matching public keys inline or by file for JWT pre-authentication on the end-user server, the admin server, or both.

**Why this priority**: The broker uses this trust anchor for the same purpose, so the change keeps configuration consistent across all trust anchors. The issue does not name this case, so it has a lower priority.

**Independent Test**: Configure JWT pre-authentication with signature verification and an inline key set. Send a request with a JWT signed by the matching private key, and confirm that the broker derives the principal. Send a request with a JWT signed by an unknown key, and confirm that the broker rejects it.

**Acceptance Scenarios**:

1. **Given** JWT pre-authentication with signature verification and an inline or file key set, **When** a request has a JWT signed by a key in that set, **Then** the broker derives the principal as it does today.
2. **Given** JWT pre-authentication with verification disabled, **When** the operator also configures an inline or file key set, **Then** startup fails, just as it does today for a key-set URI.
3. **Given** JWT pre-authentication with signature verification and no key source, **When** the broker starts, **Then** startup fails as it does today.
4. **Given** admin-server JWT pre-authentication with signature verification and an inline or file key set, and end-user JWT pre-authentication with a different key set, **When** an admin API request has a JWT signed by a key in the admin set, **Then** the admin server derives the principal; **When** it has a JWT signed only by a key in the end-user set, **Then** the admin server rejects it, because each server verifies only against its own declared keys.

---

### Edge Cases

- **No new settings**: A trust anchor with no inline or file key set behaves as it does today. This includes discovery from issuer metadata and ADR 029's reuse of the upstream key set in proxy and hybrid modes.
- **Environment-variable configuration**: An operator can supply the inline key set through the broker's existing environment-variable override for that setting. The broker validates it as it validates a value from the configuration file.
- **Keys without key identifiers**: Key selection follows the same rules as for remotely fetched key sets. A local source does not relax or tighten which keys a credential can match.
- **Mixed algorithms in one set**: A set can contain keys for different approved asymmetric algorithms. Existing algorithm restrictions, including each impersonation issuer's permitted-algorithm list, still apply.
- **Symlinked file**: Cluster platforms commonly mount secret and configuration files through relative symbolic links. The broker follows relative symbolic links to a regular file, and that file must still be inside the trust directory (FR-023). The broker refuses an absolute symbolic link inside the trust directory, even when its target is also inside. A path that resolves to a directory fails startup.
- **Symbolic link retargeted after startup**: A re-read applies the containment check again. If a symbolic link now leads outside the trust directory, the re-read fails (FR-011). The broker keeps the last valid key set and never reads the outside file.
- **Overly broad trust directory**: Setting the trust directory to the filesystem root would allow every readable file again, so startup rejects it (FR-022). Choosing a narrow directory that holds only trust material is the operator's responsibility.
- **One file for several trust anchors**: Two or more trust anchors can reference the same file. Each trust anchor validates the file and uses it on its own terms.
- **File becomes unavailable after startup**: The broker keeps the last valid key set and reports degraded health (FR-011). Once the trust anchor's maximum refresh interval has passed since the last successful read, verification for that trust anchor fails closed until a valid file returns.
- **Partial or in-progress file write**: A re-read that sees incomplete content fails validation. The broker treats it as a failed refresh and never applies a partially valid key set.
- **Removing keys from a file**: Removing a compromised key from a file that still has other keys is a valid change. The broker applies it within the refresh bounds. Emptying the file entirely is invalid (FR-007). It does not disable the trust anchor. To disable the trust anchor, the operator changes the configuration.
- **Inline set that contains a broker public key**: The broker still never accepts broker-minted tokens as client assertions. The issuer check and the startup rejection of the broker's own issuer apply to every key source.
- **Public key-set publication**: Keys from inline or file sources are verification inputs only. The broker never publishes them through its own public key-set endpoint.

## Concrete Example Journeys *(mandatory)*

All journeys share one cast. Every ID, key, coordinate, and token is synthetic. All times are 2026-10-16 UTC.

- **Broker**: `https://broker.example.com`, `local` mode. Its own issuer is `https://broker.example.com`.
- **Gateway**: agentgateway. It signs with a locally held P-256 key, `kid` `agentgateway-2026-10`, `alg` `ES256`. Its issuer value is `https://agentgateway.internal.example.com`, and no endpoint publishes metadata or keys there.
- **User**: Alice, `alice@example.com`.
- **Agent**: *Data Analysis Assistant*, `550e8400-e29b-41d4-a716-446655440000`.
- **Service**: *GitHub Production*, resource `https://api.github.com`. Alice has an active grant for the agent and an active GitHub session.

The public JWK Set used in the journeys is:

```json
{"keys":[{"kty":"EC","crv":"P-256","kid":"agentgateway-2026-10","use":"sig","alg":"ES256",
          "x":"WbbXwVYGdJoP4Xm3qCkGvBRcRvKtEfXDbWYEksDTlKg","y":"SM9vRk8mL3Fh2N1t5dAqXyZr0bPcUeTgHjIoK4sVwQE"}]}
```

### Journey 1 — An operator trusts a locally signing gateway with an inline key set (User Story 1, Scenarios 1–3; User Story 3, Scenario 8)

**Step 1 — 08:58:00: the operator configures the client-assertion trust anchor (excerpt)**

```yaml
oauth2_authorization_server:
  mode: "local"                                   # the broker's issuer is https://broker.example.com
  local:
    token_claims_expression: '{"azp": agent.id, "aud": "token-exchange-broker"}'  # subject-token claims used in Step 3
token_exchange:
  client_assertion:
    issuer_uri: "https://agentgateway.internal.example.com"  # expected iss; never contacted
    jwks: |                                        # inline key set: no discovery, no key fetch
      {"keys":[{"kty":"EC","crv":"P-256","kid":"agentgateway-2026-10","use":"sig","alg":"ES256",
                "x":"WbbXwVYGdJoP4Xm3qCkGvBRcRvKtEfXDbWYEksDTlKg","y":"SM9vRk8mL3Fh2N1t5dAqXyZr0bPcUeTgHjIoK4sVwQE"}]}
  claim_extraction:
    principal_expression: "subject_token.sub"
    agent_id_expression: "subject_token.azp"
  authorization:
    type: "cel"
    cel:
      expression: 'client_assertion.sub == "agentgateway"'   # only the gateway may exchange
```

**Step 2 — 08:58:01: the broker starts without any outbound key request**

The broker validates the inline set and logs one entry. It sends no request to `agentgateway.internal.example.com`.

```json
{"level":"INFO","msg":"trust anchor key set loaded","trust_anchor":"token_exchange.client_assertion",
 "key_source":"inline","key_count":1,"key_ids":["agentgateway-2026-10"]}
```

The log entry contains no `x` or `y` value.

**Step 3 — 09:05:00: the gateway exchanges Alice's broker token for her GitHub token**

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=eyJhbGciOiJFUzI1NiIsImtpZCI6ImJyb2tlci0xIn0.SYNTHETIC-SUBJECT.sig
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&resource=https%3A%2F%2Fapi.github.com
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=eyJhbGciOiJFUzI1NiIsImtpZCI6ImFnZW50Z2F0ZXdheS0yMDI2LTEwIn0.SYNTHETIC-CA1.sig

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store

{
  "access_token": "gho_SYNTHETICgithubtoken000000000000000",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "expires_in": 3600
}
```

Line breaks in the form body are for display only. Decoded `client_assertion` (synthetic):

```json
{
  "header": { "alg": "ES256", "kid": "agentgateway-2026-10" },
  "payload": {
    "iss": "https://agentgateway.internal.example.com",
    "sub": "agentgateway",
    "aud": "token-exchange-broker",
    "iat": 1792141500,
    "exp": 1792141800,
    "jti": "4d0b6a2e-7f3c-4a1e-9b8d-2c5e6f7a8b9c"
  }
}
```

The broker finds `kid` `agentgateway-2026-10` in the inline set and verifies the signature. It checks `iss` against `issuer_uri` and `aud` against `token_exchange.expected_audience` (default `token-exchange-broker`), exactly as for a remote key set. Decoded `subject_token` (synthetic, minted by the broker):

```json
{
  "header": { "alg": "ES256", "kid": "broker-1" },
  "payload": {
    "iss": "https://broker.example.com",
    "sub": "alice@example.com",
    "aud": "token-exchange-broker",
    "azp": "550e8400-e29b-41d4-a716-446655440000",
    "iat": 1792141080,
    "exp": 1792144680
  }
}
```

The subject token still verifies against the broker's own published keys, not against the inline set.

**Step 4 — 09:10:00: a credential signed with an unknown key is rejected**

A client assertion with header `{"alg":"ES256","kid":"rogue-2026-10"}` and otherwise identical claims is sent in the same request.

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error": "invalid_client", "error_description": "client_assertion validation failed"}
```

This is the same response an unknown remote key produces. No token is issued.

**Observable result**: the gateway exchanges tokens with no key-set endpoint in the deployment. `GET https://broker.example.com/oauth2/jwks.json` does not list `agentgateway-2026-10`.

### Journey 2 — An operator rotates a key-set file mounted from a cluster Secret (User Story 2, Scenarios 1, 3, 4, 5, 9)

**Step 1 — 08:50:00: the operator stores the public key set in a Secret and installs the chart**

```bash
kubectl create secret generic agentgateway-jwks --from-file=jwks.json=./agentgateway-jwks.json
```

```yaml
# Helm values (excerpt)
broker:
  security:
    trustMaterial:
      directory: "/etc/agentic-identity-broker/trust"   # becomes security.trust_material_dir
      sources:
        - secret:
            name: agentgateway-jwks                       # existing Secret, no custom image
            items:
              - key: jwks.json
                path: agentgateway-jwks.json
  tokenExchange:
    clientAssertion:
      issuerUri: "https://agentgateway.internal.example.com"
      jwksFile: "/etc/agentic-identity-broker/trust/agentgateway-jwks.json"
      jwksMinRefresh: "5m"                                # re-read every 5 minutes
      jwksMaxRefresh: "1h"                                # serve the last valid set for at most 1 hour after a good read
```

The kubelet projects the Secret as `/etc/agentic-identity-broker/trust/agentgateway-jwks.json` → `..data/agentgateway-jwks.json` → `..2026_10_16_08_50_00.123/agentgateway-jwks.json`. All links are relative and inside the trust directory, so startup succeeds and logs:

```json
{"level":"INFO","msg":"trust anchor key set loaded","trust_anchor":"token_exchange.client_assertion",
 "key_source":"file","path":"/etc/agentic-identity-broker/trust/agentgateway-jwks.json",
 "key_count":1,"key_ids":["agentgateway-2026-10"]}
```

**Step 2 — 09:12:00: the operator adds a new key and later removes the old one**

The operator updates the Secret so that it holds `agentgateway-2026-10` and `agentgateway-2027-01`. The kubelet swaps `..data` to a new timestamped directory. At the first re-read after the swap, 09:15:00 in this example, the broker logs:

```json
{"level":"INFO","msg":"trust anchor key set changed","trust_anchor":"token_exchange.client_assertion",
 "key_source":"file","path":"/etc/agentic-identity-broker/trust/agentgateway-jwks.json",
 "key_count":2,"added_key_ids":["agentgateway-2027-01"],"removed_key_ids":[]}
```

When a later update removes `agentgateway-2026-10`, assertions signed by that key receive the Step 4 `401 invalid_client` response of Journey 1 after the next re-read.

**Step 3 — 09:20:00: a bad Secret update fails a re-read**

An operator accidentally stores `{"keys":[` in the Secret. The re-read fails. The broker keeps the last valid set and logs:

```json
{"level":"WARN","msg":"trust anchor key set re-read failed","trust_anchor":"token_exchange.client_assertion",
 "key_source":"file","path":"/etc/agentic-identity-broker/trust/agentgateway-jwks.json",
 "error_kind":"not_a_jwk_set","last_success_at":"2026-10-16T09:15:00Z","fails_closed_at":"2026-10-16T10:15:00Z"}
```

```http
GET /health HTTP/1.1
Host: broker.example.com

HTTP/1.1 200 OK
Content-Type: application/json

{"status":"healthy","timestamp":"2026-10-16T09:20:01Z","uptime_seconds":1801,
 "components":{"trust_anchor_key_files":"degraded"}}
```

Token exchanges still succeed with the keys from the 09:15:00 read. If no valid re-read happens before 10:15:00, client-assertion verification fails closed:

```http
HTTP/1.1 500 Internal Server Error
Content-Type: application/json

{"error": "server_error", "error_description": "failed to fetch JWKS for client assertion validation"}
```

**Step 4 — 09:24:00: the operator restores the Secret**

At the first re-read after the kubelet projects the restored Secret, 09:25:00 in this example, the broker logs `trust anchor key set recovered`, and `/health` reports `"trust_anchor_key_files":"healthy"`.

**Observable result**: key rotation needs no restart, a bad update is visible as degraded health without interrupting traffic, and the broker never serves keys from a read older than one hour.

### Journey 3 — An operator's mistakes stop startup without leaking keys (User Story 3, Scenarios 1 and 3; User Story 2, Scenarios 6 and 8)

**Step 1 — two key sources on one trust anchor**

```yaml
token_exchange:
  client_assertion:
    issuer_uri: "https://agentgateway.internal.example.com"
    jwks_uri: "https://agentgateway.internal.example.com/keys"
    jwks: '{"keys":[...]}'
```

```text
token_exchange.client_assertion: only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks)
```

**Step 2 — a private key pasted as `jwks`**

The pasted JWK also has the private member `"d":"q7T1nX9vYw3mZ4pL0aR8sK2eB6cJ5hU1oN3gF7iD9tE"`.

```text
token_exchange.client_assertion.jwks: key set contains private key material; only public keys are allowed
```

Neither the error nor any log line contains the `d`, `x`, or `y` value.

**Step 3 — a key-set path outside the trust directory**

```yaml
security:
  trust_material_dir: "/etc/agentic-identity-broker/trust"
token_exchange:
  client_assertion:
    issuer_uri: "https://agentgateway.internal.example.com"
    jwks_file: "/etc/agentic-identity-broker/config.yaml"
```

```text
token_exchange.client_assertion.jwks_file: jwks_file "/etc/agentic-identity-broker/config.yaml" is outside security.trust_material_dir "/etc/agentic-identity-broker/trust"
```

The broker does not open the file. Without `security.trust_material_dir`, the same configuration fails with `token_exchange.client_assertion.jwks_file: jwks_file requires security.trust_material_dir to be set`.

**Observable result**: each mistake stops the process before it accepts traffic, names the setting, and reveals no key material.

### Journey 4 — An impersonation rule trusts the gateway's inline keys (User Story 4, Scenario 1)

Additional cast: the target agent allows scope `read`. Alice has delegated to the agent (ADR 032).

**Step 1 — rule configuration (excerpt)**

```yaml
oauth2_authorization_server:
  local:
    token_claims_expression: '{"aud": "https://api.example.com"}'
  impersonation:
    audience_prefix: "https://broker.example.com/impersonation"
    rules:
      - name: agentgateway
        roles:
          client_assertion: { expected_audience: "https://broker.example.com/impersonation", principal_expression: "client_assertion.sub" }
          actor:            { expected_audience: "https://broker.example.com/impersonation", principal_expression: "actor_token.sub" }
          subject:          { expected_audience: "https://broker.example.com/impersonation", principal_expression: "subject_token.sub" }
        trusted_issuers:
          - issuer_uri: "https://agentgateway.internal.example.com"
            jwks: '{"keys":[{"kty":"EC","crv":"P-256","kid":"agentgateway-2026-10","use":"sig","alg":"ES256","x":"WbbXwVYGdJoP4Xm3qCkGvBRcRvKtEfXDbWYEksDTlKg","y":"SM9vRk8mL3Fh2N1t5dAqXyZr0bPcUeTgHjIoK4sVwQE"}]}'
            allowed_algorithms: ["ES256"]                # an RS256 credential is rejected (Scenario 3)
            signs_roles: ["client_assertion", "actor", "subject"]
        authorization:
          type: cel
          cel: { expression: 'client_assertion.sub == "agentgateway"' }
```

**Step 2 — 09:30:00: the gateway requests a token for Alice**

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2F550e8400-e29b-41d4-a716-446655440000
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=eyJhbGciOiJFUzI1NiIsImtpZCI6ImFnZW50Z2F0ZXdheS0yMDI2LTEwIn0.SYNTHETIC-CA2.sig
&actor_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&actor_token=eyJhbGciOiJFUzI1NiIsImtpZCI6ImFnZW50Z2F0ZXdheS0yMDI2LTEwIn0.SYNTHETIC-ACTOR.sig
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Ajwt
&subject_token=eyJhbGciOiJFUzI1NiIsImtpZCI6ImFnZW50Z2F0ZXdheS0yMDI2LTEwIn0.SYNTHETIC-SUBJ.sig
&scope=read

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store

{
  "access_token": "eyJhbGciOiJFUzI1NiIsImtpZCI6ImJyb2tlci0xIn0.SYNTHETIC-IMP.sig",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "expires_in": 3600,
  "scope": "read"
}
```

All three credentials carry `iss` `https://agentgateway.internal.example.com`, `kid` `agentgateway-2026-10`, `alg` `ES256`, and `aud` `https://broker.example.com/impersonation`. The client assertion has `sub` `agentgateway`, the actor token `sub` `support-bot`, and the subject token `sub` `alice@example.com`. Decoded `access_token` (synthetic):

```json
{
  "header": { "alg": "ES256", "kid": "broker-1" },
  "payload": {
    "iss": "https://broker.example.com",
    "sub": "alice@example.com",
    "aud": "https://api.example.com",
    "iat": 1792143000,
    "exp": 1792146600,
    "agent_id": "550e8400-e29b-41d4-a716-446655440000",
    "scope": "read",
    "act": { "iss": "https://agentgateway.internal.example.com", "sub": "support-bot" }
  }
}
```

**Observable result**: impersonation works exactly as with a remote key set, and the broker made no discovery request for the gateway issuer.

### Journey 5 — A service mesh's identity JWT verifies against a key-set file (User Story 5, Scenario 1)

Additional cast: the mesh signs identity JWTs with `kid` `mesh-2026-10`, `alg` `ES256`. Its public set is projected as `/etc/agentic-identity-broker/trust/mesh-jwks.json`.

**Step 1 — configuration (excerpt)**

```yaml
security:
  trust_material_dir: "/etc/agentic-identity-broker/trust"
server:
  enduser:
    authentication:
      jwt:
        header_name: Authorization
        verification: jwks                       # signature verification stays mandatory
        jwks_file: "/etc/agentic-identity-broker/trust/mesh-jwks.json"   # re-read every 5 minutes
        expected_issuer: "https://mesh.internal.example.com"
        expected_audience: "agentic-identity-broker"
        claim_extraction:
          principal_expression: "claims.email"
          display_name_expression: "claims.name"
```

**Step 2 — 09:30:00: Alice's browser request arrives through the mesh**

```http
GET /api/me HTTP/1.1
Host: broker.example.com
Authorization: Bearer eyJhbGciOiJFUzI1NiIsImtpZCI6Im1lc2gtMjAyNi0xMCJ9.SYNTHETIC-MESH.sig

HTTP/1.1 200 OK
Content-Type: application/json

{"data": {"principal": "alice@example.com", "displayName": "Alice Example", "email": "alice@example.com"}}
```

Decoded mesh JWT (synthetic):

```json
{
  "header": { "alg": "ES256", "kid": "mesh-2026-10" },
  "payload": {
    "iss": "https://mesh.internal.example.com",
    "sub": "8c1f2e3d-4b5a-6978-8a9b-0c1d2e3f4a5b",
    "aud": "agentic-identity-broker",
    "iat": 1792142990,
    "exp": 1792143290,
    "email": "alice@example.com",
    "name": "Alice Example"
  }
}
```

**Observable result**: the broker derives Alice's principal from a JWT whose public key lives only in a mounted file. A JWT with an unknown `kid` returns `401 Unauthorized`, as it does for a remote key set.

The same configuration under `server.admin.authentication.jwt` works identically on the admin server (User Story 5, Scenario 4). That trust anchor is separate: the admin server derives no principal from a JWT signed only by a key in the end-user set, exactly as for a JWT signed by an unknown remote key.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Each in-scope trust anchor MUST accept at most one explicit key source: a remote key-set location (the existing `jwks_uri`), an inline JWK Set document, or the path of a file that contains a JWK Set document.
- **FR-002**: The in-scope trust anchors MUST be these:
  - The token-exchange client-assertion trust anchor, including its use for machine-facing approval authentication.
  - Every trusted issuer of every impersonation rule.
  - End-user JWT pre-authentication (`server.enduser.authentication.jwt`).
  - Admin JWT pre-authentication (`server.admin.authentication.jwt`). Each server's JWT pre-authentication is a separate trust anchor.
- **FR-003**: When a trust anchor declares no explicit key source, its current behavior MUST stay unchanged. This includes issuer-metadata discovery, ADR 029's reuse of the upstream key set, and the existing startup error for JWT pre-authentication with signature verification and no key source.
- **FR-004**: When a trust anchor declares an inline or file key set, the broker MUST NOT perform issuer-metadata discovery or any outbound request to get that trust anchor's keys.
- **FR-005**: Credential validation MUST give the same result for every key source. This covers key selection, algorithm restrictions, and issuer, audience, expiry, and claim checks. Only the origin of the public keys can differ.
- **FR-006**: The broker MUST accept an inline key set as JSON text in the standard JWK Set representation (RFC 7517 §5). It MUST accept that text from both the configuration file and the existing environment-variable override mechanism.
- **FR-007**: The broker MUST validate inline and file key sets at startup. Startup MUST fail with an actionable error that names the trust anchor's configuration path in any of these cases:
  - The document is not a parseable JWK Set.
  - The document contains no keys.
  - The document exceeds the size bound for remotely fetched key sets.
  - A key includes private key material.
  - A key is symmetric.
  - A key declares an algorithm outside the broker-approved asymmetric set.
  - Two keys share a key identifier.
- **FR-008**: Errors, logs, audit records, and health output MUST NOT include inline key-set content, file contents, or any part of the key material.
- **FR-009**: When one trust anchor declares more than one explicit key source, startup MUST fail with an error that names the trust anchor and the conflicting settings.
- **FR-010**: A key-set file path MUST be absolute. If the path does not exist, cannot be read, or does not resolve to a regular file after the broker follows symbolic links, startup MUST fail with an error that names the setting and the path.
- **FR-011**: After startup, the broker MUST re-read a key-set file at least once per maximum refresh interval of the trust anchor, and no more often than its minimum refresh interval. The bounds are the trust anchor's configured `jwks_min_refresh`/`jwks_max_refresh` or their existing defaults. JWT pre-authentication uses the default refresh cadence that it applies to its remote key set. Every re-read MUST pass the full validation in FR-007.
  - A valid re-read MUST replace the key set completely.
  - A failed re-read MUST keep the last valid key set. A failed re-read is one where the file is unreadable or its contents are invalid. The staleness window is the trust anchor's maximum refresh interval, measured from the last successful read: the broker MUST NOT verify with a key set read longer ago than that. This is the same upper bound that applies to a remotely fetched key set, which is never served past its next scheduled refresh. After the window expires, verification for that trust anchor fails closed until a valid re-read succeeds.
  - The broker MUST NOT apply a partially valid key set.
- **FR-012**: When a trust anchor declares an inline key set and explicit refresh bounds, startup MUST fail. Refresh bounds remain valid with `jwks_uri`, discovery, and `jwks_file`.
- **FR-013**: For JWT pre-authentication, disabling signature verification MUST be mutually exclusive with every explicit key source: URI, inline, and file.
- **FR-014**: ADR 029's self-trust protection MUST apply for every key source. In local and hybrid modes, a client-assertion trust anchor or impersonation client-assertion issuer equal to the broker's own issuer MUST fail startup. The broker MUST never accept broker-minted tokens as client assertions.
- **FR-015**: In proxy and hybrid modes, a client-assertion trust anchor can declare an inline or file key set and no issuer. In that case, the expected issuer MUST stay the upstream issuer. The trust anchor MUST verify client assertions against its declared keys only. Subject-token validation MUST keep using the upstream issuer's keys.
- **FR-016**: The broker MUST use keys from an inline or file source only for the trust anchor that declares them. It MUST NOT publish those keys through its public key-set endpoint. It MUST NOT use them for other trust anchors or for subject-token validation.
- **FR-017**: When trusted issuers share an issuer identifier but declare different key sources, each trusted issuer MUST verify credentials against only its own declared keys.
- **FR-018**: At startup, the broker MUST log one entry for each trust anchor that uses an inline or file key set. The entry MUST include the key source kind, the file path when there is one, the number of keys, and their key identifiers. When a re-read changes a file-based key set, the broker MUST log the added and removed key identifiers. When a re-read fails, the broker MUST log the failure. These logs MUST NOT include file contents.
- **FR-019**: An inline key set has no external dependency. It MUST NOT make the broker report degraded key-set health. A file-based key set MUST report degraded health after a failed re-read, through the same health reporting that remote key sets use. It MUST return to healthy after the next valid re-read.
- **FR-020**: The supported deployment package MUST let operators provide a key-set file from an existing cluster-managed secret or configuration object without a custom image. It MUST also let operators mount that file inside the configured trust directory.
- **FR-021**: The configuration reference and example configurations MUST document the following for each in-scope trust anchor:
  - All three key sources.
  - Their mutual exclusivity.
  - Their interaction with refresh bounds.
  - An example of a locally signing gateway.
  - The trust directory requirement for key-set files.
- **FR-022**: The broker MUST provide one broker-wide trust directory setting. It has no default. When the setting is unset, any trust anchor that declares a key-set file MUST fail startup with an error that names the key-set file setting and the missing trust directory. When the setting is set, it MUST be an absolute path that resolves, after symbolic links, to an existing directory other than the filesystem root. Otherwise, startup MUST fail.
- **FR-023**: Every key-set file MUST lie inside the trust directory, at startup and on every re-read. The broker MUST check both of these and MUST read nothing when either check fails:
  - **Configured path:** after the broker removes `.` and `..` segments, the path lies inside the trust directory.
  - **Resolved file:** after the broker follows every symbolic link, the file lies inside the resolved trust directory. Every symbolic link on the path MUST be relative; an absolute symbolic link fails this check even when its target is inside the trust directory.

  When the check fails, startup MUST fail with an error that names the setting, the path, and the trust directory, or the re-read MUST fail under FR-011. The containment and regular-file checks MUST apply to the same file that the broker reads.

### Domain Model

**Activity / Flow Diagram** (key-source resolution for one trust anchor at startup):

```mermaid
flowchart TD
    A["Trust anchor configuration"] --> B{"How many explicit key sources?"}
    B -->|"More than one"| X["Fail startup: conflicting key sources"]
    B -->|"None"| C["Existing behavior: discovery or upstream reuse"]
    B -->|"One"| D{"Which source?"}
    D -->|"jwks_uri"| E["Remote key set: existing fetch and refresh"]
    D -->|"jwks"| F["Parse JWK Set"]
    D -->|"jwks_file"| T{"Inside trust directory after resolving links?"}
    T -->|"No, or no trust directory"| X4["Fail startup: names setting, path, trust directory"]
    T -->|"Yes"| G["Read regular file"]
    G -->|"Missing or unreadable"| X2["Fail startup: names setting and path"]
    G --> F
    F --> H{"Valid public asymmetric key set?"}
    H -->|"No"| X3["Fail startup: names setting, no key material"]
    H -->|"Yes"| I["Verify credentials for this anchor only"]
```

**State Diagram** (file-based key set after startup, FR-011 and FR-019):

```mermaid
stateDiagram-v2
    [*] --> Current: valid at startup
    Current --> Current: re-read valid, replace set
    Current --> Degraded: re-read fails, keep last valid set
    Degraded --> Current: re-read valid, replace set
    Degraded --> FailedClosed: staleness window expires
    FailedClosed --> Current: re-read valid, replace set
```

**Entities** (things with unique identity):

- **Trust Anchor**: An operator-configured authority whose signed credentials the broker verifies. It is identified by its configuration location: the client-assertion trust anchor, an impersonation rule's trusted issuer, or JWT pre-authentication on the end-user or admin server. It owns exactly one resolved key-set source.

**Value Objects** (things without identity):

- **Key Set Source**: The origin of a trust anchor's public verification keys. It is exactly one of:
  - **Remote**: An explicit or discovered key-set URI with refresh bounds.
  - **Inline**: A JWK Set in configuration. It never changes during the process lifetime.
  - **File**: A JWK Set at an absolute path. The broker re-reads it within the trust anchor's refresh bounds (FR-011).

  It is immutable once resolved at startup and is never shared with another trust anchor's verification.
- **Local Key Set**: A validated JWK Set from an inline or file source. Its invariants are that it contains at least one key, it contains public asymmetric keys only, its key identifiers are unique, and it stays within the size bound.
- **Trust Directory**: The one operator-designated directory that holds key-set files. It is the only place from which the broker reads trust material. Its invariants are that it is absolute, resolves to an existing directory, and is not the filesystem root.

*Add "Key Set Source" and "Trust Directory" to the glossary in `ARCHITECTURE.md` (Constitution Principle V).*

### Configuration Requirements

**Configuration Parameters**:

- **`jwks`** (string, optional, no default): An inline JWK Set as JSON text. It is available on the client-assertion trust anchor (`token_exchange.client_assertion`), on each impersonation trusted issuer (`oauth2_authorization_server.impersonation.rules[].trusted_issuers[]`), and on JWT pre-authentication (`server.enduser.authentication.jwt` and `server.admin.authentication.jwt`). It is mutually exclusive with `jwks_uri` and `jwks_file`. The name matches the established OAuth2 convention that pairs an inline `jwks` value with a `jwks_uri` reference. Example environment-variable override: `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS`.
- **`jwks_file`** (string, optional, no default): The absolute path of a file that contains a JWK Set. The file must be inside `security.trust_material_dir`. It is available in the same locations as `jwks`. It is mutually exclusive with `jwks_uri` and `jwks`.
- **`security.trust_material_dir`** (string, optional, no default): The absolute path of the only directory from which the broker reads key-set files. While it is unset, `jwks_file` is unavailable. It must not be the filesystem root. Environment-variable override: `IDENTITY_BROKER_SECURITY_TRUST_MATERIAL_DIR`.
- **`jwks_min_refresh` / `jwks_max_refresh`** (existing): These remain valid with `jwks_uri` and discovery. With `jwks_file`, they bound how often the broker re-reads the file. Their defaults stay the same: 15 minutes and `max(jwks_min_refresh, 1 hour)`. They are rejected with `jwks`. JWT pre-authentication still has no configurable refresh bounds.

**Example YAML Configuration**:

```yaml
security:
  trust_material_dir: "/etc/agentic-identity-broker/trust"

token_exchange:
  client_assertion:
    issuer_uri: "https://agentgateway.internal.example.com"
    jwks: |
      {"keys":[{"kty":"EC","crv":"P-256","kid":"agentgateway-2026-10","use":"sig","alg":"ES256",
                "x":"<base64url>","y":"<base64url>"}]}

oauth2_authorization_server:
  impersonation:
    rules:
      - name: agentgateway
        trusted_issuers:
          - issuer_uri: "https://agentgateway.internal.example.com"
            jwks_file: "/etc/agentic-identity-broker/trust/agentgateway-jwks.json"
            allowed_algorithms: ["ES256"]
            signs_roles: ["client_assertion", "actor"]

server:
  enduser:
    authentication:
      jwt:
        verification: jwks
        jwks_file: "/etc/agentic-identity-broker/trust/mesh-jwks.json"
```

**Configuration Location**: Add the example to `examples/config/` and reference it in `examples/config/README.md`. Document it in `docs/configuration.md` and in the Helm chart values.

### Security Requirements

- **SR-001**: Every inline or file key set MUST be fully validated before the broker accepts traffic. A key source that fails validation MUST fail startup. It MUST never cause a fallback to discovery or to another key source.
- **SR-002**: A local key set MUST contain public asymmetric verification keys only. Private key material and symmetric keys MUST be rejected. The broker MUST NOT accept a signing secret through this mechanism.
- **SR-003**: Key parsing and signature verification MUST use the broker's existing vetted JOSE library (Constitution Principle III). Local sources MUST NOT introduce custom key parsing.
- **SR-004**: Configuration diagnostics and audit logs MUST identify the trust anchor and key identifiers without revealing key-set content (FR-008, FR-018).
- **SR-005**: The self-trust boundary from ADR 029 MUST hold for every key source (FR-014).
- **SR-006**: The broker MUST read key-set files only from inside the operator-designated trust directory (FR-022, FR-023). A trust anchor's configuration MUST NOT be able to make the broker read any other file in its runtime environment, for example credentials, configuration, or platform-provided tokens.

### Key Entities

- **Trust Anchor**: The client-assertion trust anchor, an impersonation trusted issuer, or JWT pre-authentication. It is the unit that declares one key source.
- **Key Set Source**: The remote, inline, or file origin of a trust anchor's verification keys.
- **Local Key Set**: A startup-validated JWK Set from an inline or file source.
- **Trust Directory**: The operator-designated directory that contains all key-set files.

*Add domain concepts to the glossary in `ARCHITECTURE.md` (Constitution Principle V).*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can enable token exchange for a locally signing gateway without operating any additional network-reachable service to publish its public key.
- **SC-002**: With inline or file key sets configured for every in-scope trust anchor, the broker completes startup and verifies credentials with zero outbound requests for those trust anchors' keys.
- **SC-003**: Run one credential corpus with valid, wrongly signed, expired, wrong-audience, and wrong-issuer credentials against a remote key set and against the same keys supplied inline and by file. All three key sources must produce the same accept or reject outcome for 100% of the credentials.
- **SC-004**: 100% of the invalid key-source configurations listed in FR-007, FR-009, FR-010, FR-012, FR-013, FR-022, and FR-023 must prevent startup with an error that names the offending setting. 0% of those errors and of the startup logs may contain key material.
- **SC-005**: Existing deployments that adopt no new setting must start and behave identically. This means 100% of existing token-exchange, impersonation, and JWT pre-authentication acceptance tests pass unchanged.
- **SC-006**: An operator can move a trust anchor from a remote key-set URI to an inline or file key set by changing one configuration setting.
- **SC-007**: A valid replacement of a key-set file takes effect without a restart within the trust anchor's maximum refresh interval. With default settings, this is within 1 hour for client-assertion and impersonation trust anchors. 100% of failed re-reads leave the previous valid key set in effect and are visible as degraded health.

## Assumptions

- The issue discussion between the issue author and a maintainer confirms that inline and file-based key sets are the only alternatives needed now.
- Issuer identifier rules do not change. A trust anchor still requires an issuer identifier wherever it does today, and the identifier keeps its current format rules. A locally signing gateway is configured to emit an issuer value that satisfies those rules. A local key source removes only the need for the issuer to be reachable.
- The broker accepts the inline key set as JSON text. That text can be written as a YAML block scalar or supplied through an environment variable. A structured YAML mapping is not required.
- The size bound for local key sets is the same as the existing bound for remotely fetched key sets.
- Public keys are not secrets. A key-set file can still come from a secret store, so the broker never logs key-set contents.
- ExtProc does not verify JWT signatures against an operator-configured key set, so this feature does not affect it.

## Dependencies

- **ADR 029** (token-exchange client-assertion trust anchor): extended. Before implementation, a new ADR or an amendment MUST record the key-source decision and its precedence rules (Constitution Principle II).
- **ADR 008** (token-exchange JWKS adapter pattern): local key sources MUST meet the same key-set contract that the domain consumes, so that domain validation stays source-agnostic.
- **Feature 013** (token exchange), **feature 016** (JWT pre-authentication), and **feature 037** (OAuth2 user impersonation) own the trust anchors that this feature extends.
- **Helm chart** (`charts/agentic-identity-broker/`): it needs a way to mount a key-set file from cluster-managed objects (FR-020).

## Out of Scope

- Inline or file key sets for the proxy-mode upstream authorization server. Its keys validate subject tokens, and the broker republishes them. That server is a full authorization server with discovery.
- Combining or merging several key sources for one trust anchor, for example remote and inline keys together.
- Per-agent or per-client verification keys supplied through the administrative API or through client metadata documents.
- Supplying the broker's own signing keys or any private key through configuration.
- Symmetric (shared-secret) signature verification.
- Relaxing issuer identifier format or reachability rules.
- Changes to the standalone ExtProc service.
- Four other key-set file controls were considered on 2026-10-12 and not adopted. The broker does not reject a file because it is writable. It does not refuse pseudo-filesystem paths beyond the trust-directory check, and it does not tighten error wording beyond FR-008. It does not record content digests in the audit log. It does not limit mounts to the deployment chart.
- Reloading inline key sets without a restart.
