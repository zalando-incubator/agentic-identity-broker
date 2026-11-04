# ADR 046: Observed Refresh-Token Rejection

**Status**: Accepted
**Date**: 2026-11-02
**Supersedes**: Only ADR 038's availability consequence for a terminal provider refresh rejection.

## Context

The broker cannot predict the lifetime of an opaque third-party refresh token. The
`user_sessions.refresh_token_expires_at` column was created for a provider-reported expiry, but no
code path ever wrote a provider value. Since PR #271, the only write is the moment a provider
answers a refresh with HTTP 4xx `invalid_grant`. The session API and the sessions page still
presented that value as a refresh-token expiry, with an "Expiring Soon" status and a predicted
date, and token exchange kept issuing a still-valid access token after the rejection.

PR #224 proposed a boolean column behind a three-phase online migration with a SQL view, write
functions, and Helm rollout guards. Its preparation phase could not persist a rejection on
PostgreSQL until the second phase shipped, which reintroduced repeated provider calls and replaced
the reauthentication signal with a persistence error. This ADR keeps the behavioral goal and drops
the schema machinery.

## Decision

- The domain field is `UserSession.RefreshRejectedAt`. A non-nil value records when the provider
  terminally rejected the stored refresh token. Nil means no rejection has been observed; it does
  not prove validity. The value is stored in the existing `refresh_token_expires_at` column and is
  never exposed as an expiry. No schema migration changes the column.
- The broker records a rejection only for HTTP 4xx `invalid_grant`, excluding 429, through the
  existing locked update-only refresh callback. Ciphertext, access expiry, token type, and scope
  stay unchanged. A successful authorization callback or a rotated refresh token clears it.
- `CanRefresh` is a stored refresh token without a recorded rejection.
  `RequiresReauthentication` is a recorded rejection, or an expired access token without a stored
  refresh token. A recorded rejection refuses further broker token issuance even while the stored
  access token has not expired. Background and sweep work skip rejected sessions; the sweep
  candidate query excludes them.
- The session API exposes `requires_reauthentication` and keeps `is_expired` as a deprecated alias
  with the same value. `refresh_token_expires_at` is removed from every response. The detail
  endpoint returns the summary plus ownership and lifecycle metadata. Manual refresh of a rejected
  session, and a live terminal rejection during manual refresh, return 409 `refresh_rejected`.
  Token exchange keeps 400 `invalid_grant` with the ADR 044 recovery `error_uri`.
- The sessions page shows "Login required" with the hint "Log in to reconnect", offers Login, keeps
  Terminate for every session, and never shows a predicted refresh expiry.

## Consequences

- A rejection stops future broker issuance from the moment it is committed. It does not revoke
  already-issued broker tokens or ExtProc caches; those expire with the access token.
- One release ships the change. In-memory and PostgreSQL adapters behave identically, and a
  rollback to the previous binary reads the same column with its previous meaning.
- Operators and users see one reauthentication state instead of a fabricated expiry date.

## Alternatives Considered

- **Boolean column with online view and function boundary (PR #224 to #226)**: Rejected. The
  machinery served a column rename, left PostgreSQL unable to persist rejections for one release,
  and coupled unrelated Helm rollout defaults into a session-semantics change.
- **Keep serving a valid access token after a rejection**: Rejected. A provider `invalid_grant`
  usually means the grant was revoked. With a long sweep lookahead the broker could otherwise keep
  issuing a token for hours after learning that the user must reauthenticate.

## References

- `adrs/038-proactive-token-refresh.md`
- `adrs/044-service-specific-session-recovery-url.md`
- `specs/008-thirdparty-oauth2-sessions/spec.md`
- `specs/013-token-exchange/spec.md`
