# Changelog

## Unreleased — consent-bound local refresh sessions

The broker implements the contract approved on 2026-10-01 in [feature 049](../specs/049-fix-refresh-consent/spec.md).
This entry does not assign a release version or claim a shipped release.

- Local user refresh sessions on `POST /oauth2/token` require the original active
  consent throughout refresh, including retries. Missing or expired consent,
  terminal or expired sessions, and prohibited token reuse return generic,
  token-free `400 invalid_grant` without `error_uri`. Unknown authorization or
  session state and failed commits return token-free `500 server_error`.
- Successful fresh refresh rotates the token. A narrower requested scope limits
  only the returned access token. The session keeps its original scope ceiling.
  The default reuse interval changes to `30s`. Only the immediate predecessor
  can return an identical token pair within its fixed window while its successor
  remains unused and its original access token remains valid. At most three
  committed stored-result returns are permitted per consumed token. A retry's
  `expires_in` reports remaining validity, not a new access-token lifetime.
  Explicit `0s` reuse restores strict single-use behavior.
- Confirmed pre-commit failure or rollback leaves a token unconsumed. An
  indeterminate commit returns `server_error` without a rollback claim. Later
  requests resolve durable state under normal authorization and retry rules.
  Committed rotation yields only its existing eligible result. Zero reuse
  cannot recover committed consumption. Recovery never creates a second
  successor or refunds a committed retry count.
- The default absolute lifetime is unlimited (`0s`). The inactivity lifetime
  defaults to `720h` (30 days) through the existing
  `oauth2_authorization_server.local.refresh_token_ttl` setting. Fresh rotation
  alone renews inactivity. Neither retry grace nor a policy increase restores
  an expired session.
- Existing consent deletion, agent deletion, and explicit credential revocation
  also end their related local refresh sessions. Empty grant submission revokes
  sessions even if its grant is already absent. Renewal after an expired grant
  revokes its older sessions. Updating an active grant retains them. **Credential
  replacement is different from revocation**: it keeps sessions but requires
  the replacement credential for later refresh. Authorization-code replay
  remains a separate revocation exception, even for a replaying client other
  than the session's client. Existing access JWTs keep their issued expiry
  at signature-only resource servers.
- Every pre-feature unanchored local session and unsupported old-writer
  descendant requires fresh authorization. An evidenced anchored family from
  a new issuer can continue only under current authorization. After a full
  database or refresh-state restore, token traffic and writers stay stopped
  until the offline restore-invalidation command succeeds. Encrypted backup
  copies cannot restore refresh authority.
- Rolling deployment retains the additive refresh-session migrations and the legacy
  `refresh_token_sessions` table. Before binary-only rollback, stop token traffic
  and all old writers. Complete expiry reconciliation under the outgoing policy
  before the old binary receives traffic. If reconciliation fails, keep traffic
  stopped. Neither binary-only rollback nor migration down/reapply restores terminal
  authority. Follow the [Upgrade and Binary-Only Rollback procedure](deployment/kubernetes.md#upgrade-and-binary-only-rollback).
- Committed local-session revocation and lifetime expiry emit per-session structured
  audits with trusted, redacted context. Independent receipts retain this context
  before cascade deletion. Confirmed rollback emits no successful transition.
  Known-token client mismatches preserve owner identity only for rejection auditing.
  Migration 037 maintains an indexed lineage proof so fresh/retry eligibility does
  not traverse history; missing or inconsistent intermediate evidence fails closed.
- Startup reconciles effective session and retry deadlines before readiness.
  Live maintenance erases expired retry ciphertext within one second with reachable storage.
  Storage or cleanup errors block readiness until recovery. Encrypted backups can retain older copies.
- The offline `refresh-sessions invalidate-restored` command starts no HTTP server.
  It requires acknowledged invalidation and a final empty-authority scan before admission.
  Failed or indeterminate completion requires an idempotent rerun with all writers stopped.

This changes **existing endpoints and their authorization behavior**, not their
paths or wire response members. `POST /oauth2/token` retains its `tokenExchange`
operation ID and existing success/error status conventions. Admin and consent
lifecycle actions retain their authentication, ownership, and not-found rules.
Authorization-server metadata remains unchanged. It does not advertise a
revocation or introspection endpoint. The change applies only to broker-issued
local user sessions in `local` mode and the local path of `hybrid` mode. Upstream
and vaulted third-party refresh behavior remains unchanged.

The owner approved **major-release handling for any released-contract break**
under the constitution's API-version rule. Do not silently change a deployed
contract under its previous release version, add a compatibility bypass, or
assign a release number from this entry. API examples and
error/recovery rules are in the [refresh-session guide](api/oauth2-refresh-sessions.md)
and the existing OpenAPI documents.

### Validation and release status

All 47 feature acceptance scenarios passed, including real PostgreSQL and browser journeys.
Static checks, race-enabled Go tests, frontend unit tests, migrations, and restore-command checks also passed.
The complete browser suite passed in serial mode after two navigation timeouts in its initial parallel run.
The [feature validation record](../specs/049-fix-refresh-consent/plan.md#validation-record) preserves both outcomes and the non-clean quality-delta assessment.

Convergence verification observed passing outcomes for all 47 primary scenarios across backend, isolated time-sensitive, and browser runs.
Its combined backend run passed 43 of 46 cases before three unchanged scenarios passed separately.
That combined run is not a passing gate. The scoped source-security scan reported zero issues.
The validation record retains incomplete broad checks and the non-clean scoped quality assessment.

Full `just verify` remains blocked by `GO-2026-6443` in the existing `google.golang.org/grpc v1.84.0` dependency.
The owner retained that pin instead of approving a prerelease upgrade.
No security check was bypassed. This feature is not declared release-ready.
