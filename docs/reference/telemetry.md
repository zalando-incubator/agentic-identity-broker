---
title: "Telemetry"
description: Spans, metrics and attributes that the broker and ExtProc emit through OpenTelemetry. Generated from the telemetry registry.
---

# Telemetry

This page lists every span, metric and attribute that the broker and ExtProc create through
the OpenTelemetry API. It is generated from the telemetry registry in `telemetry/registry`
with [OpenTelemetry Weaver](https://github.com/open-telemetry/weaver). Do not edit it by
hand. After you change a span or a metric, change the registry and run `just telemetry-doc`.
`just telemetry-check` fails when the page is out of date.

The registry is written by hand. CI runs the backend E2E, ExtProc E2E and PostgreSQL
integration suites, and compares the telemetry that they emit with the registry. It reports
a span, a metric, an attribute or a value that the tests emit and the registry does not
list. It does not check a span or a metric that no test emits, and it does not report a
registry entry that the code no longer emits.

The page does not list:

- the HTTP server spans and metrics that `otelchi` creates for each inbound request;
- metrics that the OpenTelemetry SDK, the Go runtime or a library creates;
- span events and resource attributes;
- log records. `coding-standards/LOGGING.md` covers them.

A value list shows the values that the span or the metric sets. `any` means that the value is
not one of a fixed set. Rules for adding telemetry are in `coding-standards/OPENTELEMETRY.md`.

## Spans

Error status: whether the span can end with status `Error`.

| Span | Kind | Scope | Error status | Description |
|---|---|---|---|---|
| [`approval.approve`](#approvalapprove) | Internal | `approval` | No | Transitions a pending approval to approved. |
| [`approval.consume`](#approvalconsume) | Internal | `approval` | No | Marks an approved approval with persistence `once` as consumed. |
| [`approval.created`](#approvalcreated) | Internal | `approval` | No | Creates a pending approval, or returns the existing one. |
| [`approval.deny`](#approvaldeny) | Internal | `approval` | No | Transitions a pending approval to denied. |
| [`approval.list_pending`](#approvallist_pending) | Internal | `approval` | No | Lists the pending approvals of a principal. |
| [`approval.list_permanent`](#approvallist_permanent) | Internal | `approval` | No | Lists the permanent approvals and denials of a principal. |
| [`approval.review`](#approvalreview) | Internal | `approval` | No | Retrieves an approval by ID for the approval view. |
| [`approval.revoke`](#approvalrevoke) | Internal | `approval` | No | Transitions a permanent approval to denied. |
| [`approval.sync`](#approvalsync) | Internal | `approval` | No | Returns the sync version and the active approvals, grouped by principal and agent. |
| [`encryption.decrypt`](#encryptiondecrypt) | Internal | `encryption` | No | Decrypts ciphertext with AWS KMS envelope encryption. |
| [`encryption.encrypt`](#encryptionencrypt) | Internal | `encryption` | No | Encrypts plaintext with AWS KMS envelope encryption. |
| [`extproc.approval.gate`](#extprocapprovalgate) | Internal | `extproc` | No | Approval gate evaluation in ExtProc. |
| [`extproc.client_assertion.refresh`](#extprocclient_assertionrefresh) | Internal | `extproc` | Yes | Obtains a new client assertion through the client credentials grant. |
| [`extproc.opa.evaluate`](#extprocopaevaluate) | Internal | `extproc` | No | Evaluates the OPA policy for a request. |
| [`extproc.token_exchange`](#extproctoken_exchange) | Server | `extproc` | Yes | Token exchange in ExtProc. |
| [`http.request`](#httprequest) | Client | `telemetryhttp` | Yes | Outbound HTTP call. |
| [`jwks.fetch`](#jwksfetch) | Internal | `jwks` | Yes | JWKS key set fetch or cache lookup. |
| [`oauth2.token_proxy`](#oauth2token_proxy) | Internal | `upstream` | No | Upstream OAuth2 token proxy. |
| [`oauth2session.background_refresh`](#oauth2sessionbackground_refresh) | Internal | `oauth2session` | No | Third-party session refresh in the background. |
| [`storage.approve.tool_approval`](#storageapprovetool_approval) | Client | `storage` | Yes | Transitions a pending tool approval to approved in PostgreSQL. |
| [`storage.consume.client_assertion_replay`](#storageconsumeclient_assertion_replay) | Client | `storage` | Yes | Records the `jti` of a client assertion, or reports a conflict when it is already recorded. |
| [`storage.consume.tool_approval`](#storageconsumetool_approval) | Client | `storage` | Yes | Marks an approved tool approval with persistence `once` as consumed in PostgreSQL. |
| [`storage.count_pending.tool_approval`](#storagecount_pendingtool_approval) | Client | `storage` | Yes | Counts the pending approvals of a principal and agent pair that are not expired. |
| [`storage.create.authorization_code`](#storagecreateauthorization_code) | Client | `storage` | Yes | Inserts an authorization code. |
| [`storage.create.client_credential`](#storagecreateclient_credential) | Client | `storage` | Yes | Inserts the client credential of an agent. |
| [`storage.create.pkce_session`](#storagecreatepkce_session) | Client | `storage` | Yes | Inserts a PKCE session. |
| [`storage.create.refresh_token_session`](#storagecreaterefresh_token_session) | Client | `storage` | Yes | Inserts a refresh token session. |
| [`storage.create.tool_approval`](#storagecreatetool_approval) | Client | `storage` | Yes | Inserts a tool approval, or returns the existing pending duplicate. |
| [`storage.create.user_grant`](#storagecreateuser_grant) | Client | `storage` | Yes | Creates a user grant, or updates the grant of the same principal and agent. |
| [`storage.delete.client_credential`](#storagedeleteclient_credential) | Client | `storage` | Yes | Deletes the client credential of an agent. |
| [`storage.delete.pkce_session`](#storagedeletepkce_session) | Client | `storage` | Yes | Deletes a PKCE session by its signature. |
| [`storage.delete_expired.authorization_code`](#storagedelete_expiredauthorization_code) | Client | `storage` | Yes | Deletes the expired authorization codes. |
| [`storage.delete_expired.client_assertion_replay`](#storagedelete_expiredclient_assertion_replay) | Client | `storage` | Yes | Deletes the expired client assertion replay records. |
| [`storage.delete_expired.pkce_session`](#storagedelete_expiredpkce_session) | Client | `storage` | Yes | Deletes the expired PKCE sessions. |
| [`storage.delete_expired.refresh_token_session`](#storagedelete_expiredrefresh_token_session) | Client | `storage` | Yes | Deletes the expired refresh token sessions. |
| [`storage.deny.tool_approval`](#storagedenytool_approval) | Client | `storage` | Yes | Transitions a pending tool approval to denied in PostgreSQL. |
| [`storage.find.authorization_code`](#storagefindauthorization_code) | Client | `storage` | Yes | Retrieves an authorization code that is not expired by its code hash. |
| [`storage.find.pkce_session`](#storagefindpkce_session) | Client | `storage` | Yes | Retrieves a PKCE session by its signature. |
| [`storage.find.refresh_token_session`](#storagefindrefresh_token_session) | Client | `storage` | Yes | Retrieves a refresh token session by its signature. |
| [`storage.get.agent`](#storagegetagent) | Client | `storage` | Yes | Retrieves an agent by ID from PostgreSQL. |
| [`storage.get.client_credential`](#storagegetclient_credential) | Client | `storage` | Yes | Retrieves the client credential of an agent. |
| [`storage.get.tool_approval`](#storagegettool_approval) | Client | `storage` | Yes | Retrieves a tool approval by ID from PostgreSQL. |
| [`storage.get_version.approval_sync`](#storageget_versionapproval_sync) | Client | `storage` | Yes | Retrieves the current approval sync version. |
| [`storage.increment_version.approval_sync`](#storageincrement_versionapproval_sync) | Client | `storage` | Yes | Increments the approval sync version and issues `NOTIFY`. |
| [`storage.list.user_grants_by_principal`](#storagelistuser_grants_by_principal) | Client | `storage` | Yes | Retrieves the active grants of a principal across all agents. |
| [`storage.list_active.tool_approval`](#storagelist_activetool_approval) | Client | `storage` | Yes | Lists the active approvals that are not expired and not consumed. |
| [`storage.list_permanent.tool_approval`](#storagelist_permanenttool_approval) | Client | `storage` | Yes | Lists the permanent approvals and denials of a principal. |
| [`storage.mark_used.authorization_code`](#storagemark_usedauthorization_code) | Client | `storage` | Yes | Marks an unused authorization code as used. |
| [`storage.mark_used.refresh_token_session`](#storagemark_usedrefresh_token_session) | Client | `storage` | Yes | Marks an unused refresh token session as used. |
| [`storage.revoke_by_agent.refresh_token_session`](#storagerevoke_by_agentrefresh_token_session) | Client | `storage` | Yes | Revokes the unused refresh token sessions of an agent. |
| [`storage.revoke_by_principal_and_agent.refresh_token_session`](#storagerevoke_by_principal_and_agentrefresh_token_session) | Client | `storage` | Yes | Revokes the unused refresh token sessions of a principal and agent pair. |
| [`storage.revoke_by_request.refresh_token_session`](#storagerevoke_by_requestrefresh_token_session) | Client | `storage` | Yes | Revokes the unused refresh token sessions of a token request. |
| [`storage.revoke_permanent.tool_approval`](#storagerevoke_permanenttool_approval) | Client | `storage` | Yes | Clears a permanent approval or denial. |
| [`storage.rotate.client_credential`](#storagerotateclient_credential) | Client | `storage` | Yes | Replaces the client credential of an agent in one transaction. |
| [`tokenexchange.exchange`](#tokenexchangeexchange) | Internal | `tokenexchange` | Yes | RFC 8693 token exchange request in the broker. |
| [`tokenexchange.get_valid_access_token`](#tokenexchangeget_valid_access_token) | Internal | `tokenexchange` | No | Gets a valid access token for the session, and refreshes it when a refresh token is available. |
| [`tokenexchange.impersonation`](#tokenexchangeimpersonation) | Internal | `tokenexchange` | Yes | RFC 8693 impersonation request in the broker. |
| [`tokenexchange.verify_agent_access`](#tokenexchangeverify_agent_access) | Internal | `tokenexchange` | No | Checks that the principal has an active grant for the agent. |

## Metrics

| Metric | Instrument | Unit | Scope | Description |
|---|---|---|---|---|
| [`approvals_pending_total`](#approvals_pending_total) | updowncounter | `{approval}` | `approval` | Number of pending tool approvals. |
| [`extproc.approval.gate.decisions`](#extprocapprovalgatedecisions) | counter |  | `extproc` | Approval gate decisions in ExtProc. |
| [`extproc.approval.sync_age`](#extprocapprovalsync_age) | gauge | `s` | `extproc` | Seconds since the last successful approval sync. |
| [`extproc.token_exchange.duration`](#extproctoken_exchangeduration) | histogram | `s` | `extproc` | Duration of token exchange requests in seconds. |
| [`extproc.token_exchange.requests`](#extproctoken_exchangerequests) | counter |  | `extproc` | Total number of token exchange requests processed by ExtProc. |
| [`long_poll_connections_active`](#long_poll_connections_active) | updowncounter | `{connection}` | `approval` | Number of active long-poll connections. |
| [`oauth2_sessions`](#oauth2_sessions) | gauge | `{session}` | `oauth2session` | Stored third-party sessions by service, access-token expiry class and refreshability. |
| [`proactive_refresh_dropped_total`](#proactive_refresh_dropped_total) | counter | `{refresh}` | `oauth2session` | Background session refreshes dropped because the pool is full. |
| [`proactive_refresh_failed_total`](#proactive_refresh_failed_total) | counter | `{refresh}` | `oauth2session` | Admitted background session refreshes that failed. |
| [`proactive_refresh_triggered_total`](#proactive_refresh_triggered_total) | counter | `{refresh}` | `oauth2session` | Background session refreshes admitted to the pool. |
| [`session_sweep_duration_seconds`](#session_sweep_duration_seconds) | histogram | `s` | `oauth2session` | Duration of admin session sweep requests by outcome. |
| [`session_sweep_failed_total`](#session_sweep_failed_total) | counter | `{refresh}` | `oauth2session` | Admin sweep candidates that could not be refreshed. |
| [`session_sweep_refreshed_total`](#session_sweep_refreshed_total) | counter | `{refresh}` | `oauth2session` | Sessions refreshed and committed by an admin sweep. |
| [`session_sweep_skipped_total`](#session_sweep_skipped_total) | counter | `{refresh}` | `oauth2session` | Admin sweep candidates skipped because they were already refreshed or deleted. |

## Span attributes

### approval.approve

Transitions a pending approval to approved.

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.persistence` | `once`, `permanent`, `session` |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |
| `approval.tool_pattern` | any `string` |

### approval.consume

Marks an approved approval with persistence `once` as consumed.

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.tool_name` | any `string` |

### approval.created

Creates a pending approval, or returns the existing one.

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.is_new` | any `boolean` |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |

### approval.deny

Transitions a pending approval to denied.

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |

### approval.list_pending

Lists the pending approvals of a principal.

| Attribute | Values |
|---|---|
| `approval.count` | any `int` |
| `approval.principal` | any `string` |

### approval.list_permanent

Lists the permanent approvals and denials of a principal.

| Attribute | Values |
|---|---|
| `approval.count` | any `int` |
| `approval.principal` | any `string` |

### approval.review

Retrieves an approval by ID for the approval view.

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.principal` | any `string` |
| `approval.status` | `approved`, `denied`, `pending` |
| `approval.tool_name` | any `string` |

### approval.revoke

Transitions a permanent approval to denied.

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.tool_name` | any `string` |

### approval.sync

Returns the sync version and the active approvals, grouped by principal and agent.

| Attribute | Values |
|---|---|
| `approval.pair_count` | any `int` |
| `approval.sync_version` | any `int` |

### encryption.decrypt

Decrypts ciphertext with AWS KMS envelope encryption.

| Attribute | Values |
|---|---|
| `encryption.key_type` | `aws_kms` |

### encryption.encrypt

Encrypts plaintext with AWS KMS envelope encryption.

| Attribute | Values |
|---|---|
| `encryption.key_type` | `aws_kms` |

### extproc.approval.gate

Approval gate evaluation in ExtProc.

| Attribute | Values |
|---|---|
| `approval.outcome` | `deny`, `elicit`, `proceed` |

### extproc.client_assertion.refresh

Obtains a new client assertion through the client credentials grant.

| Attribute | Values |
|---|---|
| `error.type` | `assertion_expired`, `caller_canceled`, `circuit_open`, `configuration`, `internal`, `invalid_response`, `none`, `rejected`, `unavailable` |
| `http.response.status_code` | any `int` |
| `token_exchange.dependency` | `broker`, `local`, `oauth_provider` |
| `token_exchange.exchange_kind` | `impersonation`, `third_party`, `unknown` |
| `token_exchange.failure_detail` | `authorization_denied`, `broker_response_invalid`, `broker_unavailable`, `caller_canceled`, `circuit_open`, `client_expired`, `client_invalid`, `configuration`, `consent_required`, `credential_rejected`, `internal_unclassified`, `none`, `policy_configuration`, `policy_denied`, `policy_evaluation_failed`, `provider_response_invalid`, `provider_unavailable`, `reauth_required`, `request_malformed`, `resource_missing`, `response_write_failed`, `session_reauth_required`, `subject_invalid` |
| `token_exchange.failure_stage` | `client_authorization`, `exchange_routing`, `none`, `refresh`, `request_validation`, `response_write` |
| `token_exchange.oauth_error_code` | any `string` |
| `token_exchange.operation` | `assertion_refresh`, `configuration`, `exchange` |
| `token_exchange.outcome` | `authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success` |
| `token_exchange.recovery_action` | `fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry` |
| `token_exchange.recovery_target` | `consent`, `none`, `provider_session` |

### extproc.opa.evaluate

Evaluates the OPA policy for a request.

| Attribute | Values |
|---|---|
| `authorization.action` | `deny`, any `string` |
| `authorization.duration_ms` | any `int` |
| `authorization.method` | any `string` |
| `authorization.protocol` | any `string` |
| `authorization.result_code` | `error`, `ok`, `timeout`, `undefined`, `undefined_action`, `unsupported_action` |
| `authorization.target_server_name` | any `string` |
| `authorization.tool_name` | any `string` |

### extproc.token_exchange

Token exchange in ExtProc.

| Attribute | Values |
|---|---|
| `error.type` | `assertion_expired`, `caller_canceled`, `circuit_open`, `configuration`, `internal`, `invalid_response`, `none`, `rejected`, `unavailable` |
| `token_exchange.broker_status_code` | any `int` |
| `token_exchange.dependency` | `broker`, `local`, `oauth_provider` |
| `token_exchange.exchange_kind` | `impersonation`, `third_party`, `unknown` |
| `token_exchange.failure_detail` | `authorization_denied`, `broker_response_invalid`, `broker_unavailable`, `caller_canceled`, `circuit_open`, `client_expired`, `client_invalid`, `configuration`, `consent_required`, `credential_rejected`, `internal_unclassified`, `none`, `policy_configuration`, `policy_denied`, `policy_evaluation_failed`, `provider_response_invalid`, `provider_unavailable`, `reauth_required`, `request_malformed`, `resource_missing`, `response_write_failed`, `session_reauth_required`, `subject_invalid` |
| `token_exchange.failure_stage` | `client_authorization`, `exchange_routing`, `none`, `refresh`, `request_validation`, `response_write` |
| `token_exchange.oauth_error_code` | any `string` |
| `token_exchange.operation` | `assertion_refresh`, `configuration`, `exchange` |
| `token_exchange.outcome` | `authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success` |
| `token_exchange.recovery_action` | `fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry` |
| `token_exchange.recovery_target` | `consent`, `none`, `provider_session` |

### http.request

Outbound HTTP call. The span does not record URLs, headers, bodies or errors.

| Attribute | Values |
|---|---|
| `error.type` | `network_error`, `response_close_failed`, `response_read_failed` |
| `http.request.method` | any `string` |
| `http.response.status_code` | any `int` |

### jwks.fetch

JWKS key set fetch or cache lookup.

| Attribute | Values |
|---|---|
| `error.type` | `jwks_unavailable` |

### oauth2.token_proxy

Upstream OAuth2 token proxy.

| Attribute | Values |
|---|---|
| `http.request.method` | `POST`, any `string` |
| `http.response.status_code` | any `int` |

### oauth2session.background_refresh

Third-party session refresh in the background. A new root span, with a link to the span of the request that triggered the refresh.

| Attribute | Values |
|---|---|
| `oauth2.refresh.outcome` | `failed`, `not_due`, `refreshed` |
| `oauth2.refresh.triggered_by` | `background` |

### storage.approve.tool_approval

Transitions a pending tool approval to approved in PostgreSQL.

| Attribute | Values |
|---|---|
| `db.operation` | `ApproveToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.consume.client_assertion_replay

Records the `jti` of a client assertion, or reports a conflict when it is already recorded.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientAssertionReplayRepo.Consume` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.consume.tool_approval

Marks an approved tool approval with persistence `once` as consumed in PostgreSQL.

| Attribute | Values |
|---|---|
| `db.operation` | `ConsumeToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.count_pending.tool_approval

Counts the pending approvals of a principal and agent pair that are not expired.

| Attribute | Values |
|---|---|
| `db.operation` | `CountPendingToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.create.authorization_code

Inserts an authorization code.

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.create.client_credential

Inserts the client credential of an agent.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.create.pkce_session

Inserts a PKCE session.

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.create.refresh_token_session

Inserts a refresh token session.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.create.tool_approval

Inserts a tool approval, or returns the existing pending duplicate.

| Attribute | Values |
|---|---|
| `db.operation` | `CreateToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.create.user_grant

Creates a user grant, or updates the grant of the same principal and agent.

| Attribute | Values |
|---|---|
| `db.operation` | `CreateUserGrant` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.delete.client_credential

Deletes the client credential of an agent.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Delete` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.delete.pkce_session

Deletes a PKCE session by its signature.

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.Delete` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.delete_expired.authorization_code

Deletes the expired authorization codes.

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.delete_expired.client_assertion_replay

Deletes the expired client assertion replay records.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientAssertionReplayRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.delete_expired.pkce_session

Deletes the expired PKCE sessions.

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.delete_expired.refresh_token_session

Deletes the expired refresh token sessions.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.deny.tool_approval

Transitions a pending tool approval to denied in PostgreSQL.

| Attribute | Values |
|---|---|
| `db.operation` | `DenyToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.find.authorization_code

Retrieves an authorization code that is not expired by its code hash.

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.FindByCodeHash` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.find.pkce_session

Retrieves a PKCE session by its signature.

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.FindBySignature` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.find.refresh_token_session

Retrieves a refresh token session by its signature.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.FindBySignature` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.get.agent

Retrieves an agent by ID from PostgreSQL.

| Attribute | Values |
|---|---|
| `db.operation` | `GetAgent` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.get.client_credential

Retrieves the client credential of an agent.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.GetByAgentID` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.get.tool_approval

Retrieves a tool approval by ID from PostgreSQL.

| Attribute | Values |
|---|---|
| `db.operation` | `GetToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.get_version.approval_sync

Retrieves the current approval sync version.

| Attribute | Values |
|---|---|
| `db.operation` | `GetApprovalSyncVersion` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.increment_version.approval_sync

Increments the approval sync version and issues `NOTIFY`.

| Attribute | Values |
|---|---|
| `db.operation` | `IncrementApprovalSyncVersion` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.list.user_grants_by_principal

Retrieves the active grants of a principal across all agents.

| Attribute | Values |
|---|---|
| `db.operation` | `ListByPrincipal` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.list_active.tool_approval

Lists the active approvals that are not expired and not consumed.

| Attribute | Values |
|---|---|
| `db.operation` | `ListAllActiveToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.list_permanent.tool_approval

Lists the permanent approvals and denials of a principal.

| Attribute | Values |
|---|---|
| `db.operation` | `ListPermanentToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.mark_used.authorization_code

Marks an unused authorization code as used.

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.MarkUsed` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.mark_used.refresh_token_session

Marks an unused refresh token session as used.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.MarkUsed` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.revoke_by_agent.refresh_token_session

Revokes the unused refresh token sessions of an agent.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByAgent` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.revoke_by_principal_and_agent.refresh_token_session

Revokes the unused refresh token sessions of a principal and agent pair.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByPrincipalAndAgent` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.revoke_by_request.refresh_token_session

Revokes the unused refresh token sessions of a token request.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByRequestID` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.revoke_permanent.tool_approval

Clears a permanent approval or denial.

| Attribute | Values |
|---|---|
| `db.operation` | `RevokePermanentToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### storage.rotate.client_credential

Replaces the client credential of an agent in one transaction.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Rotate` |
| `db.system` | `postgresql` |
| `error.type` | `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |
| `storage.outcome` | `canceled`, `conflict`, `connection`, `not_found`, `timeout`, `unknown`, `validation` |

### tokenexchange.exchange

RFC 8693 token exchange request in the broker.

| Attribute | Values |
|---|---|
| `impersonation.target_agent_id` | any `string` |
| `token_exchange.agent.id` | any `string` |
| `token_exchange.exchange_kind` | `impersonation`, `third_party` |
| `token_exchange.failure_detail` | `access_token_expired`, `agent_invalid`, `agent_missing`, `agent_repository_unavailable`, `caller_canceled`, `cel_configuration`, `cel_evaluation_failed`, `client_expired`, `client_invalid`, `client_policy_denied`, `credential_identity_changed`, `credential_source_unavailable`, `grant_empty`, `grant_expired`, `grant_missing`, `grant_permission_set_missing`, `grant_permission_set_undeclared`, `grant_repository_unavailable`, `grant_scope_intersection_empty`, `grant_service_definition_missing`, `grant_service_omitted`, `grant_service_requirement_excluded`, `internal_unclassified`, `jwks_unavailable`, `jwt_configuration`, `none`, `provider_client_rejected`, `provider_rejected`, `provider_response_invalid`, `provider_unavailable`, `refresh_rejected`, `refresh_unavailable`, `request_malformed`, `resource_ambiguous`, `resource_missing`, `resource_repository_unavailable`, `resource_unregistered`, `response_write_failed`, `session_configuration`, `session_decryption_failed`, `session_encryption_failed`, `session_missing`, `session_persistence_failed`, `session_repository_unavailable`, `session_scope_insufficient`, `subject_category_ineligible`, `subject_category_unsupported`, `subject_expired`, `subject_invalid` |
| `token_exchange.failure_stage` | [13 values](#token_exchangefailure_stage) |
| `token_exchange.grant.id` | any `string` |
| `token_exchange.grant.updated_at` | any `string` |
| `token_exchange.grant.valid_until` | any `string` |
| `token_exchange.outcome` | `authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success` |
| `token_exchange.recovery_action` | `fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry` |
| `token_exchange.recovery_target` | `broker_configuration`, `calling_client`, `consent`, `none`, `provider_session`, `subject_identity` |
| `token_exchange.service.id` | any `string` |
| `token_exchange.session.dependency` | `credential_source`, `encryption`, `none`, `provider`, `provider_repository`, `session_repository`, `signing` |
| `token_exchange.session.detail` | [16 values](#token_exchangesessiondetail) |
| `token_exchange.session.kind` | `canceled`, `configuration`, `infrastructure`, `internal`, `provider`, `session` |
| `token_exchange.session.oauth_code` | any `string` |
| `token_exchange.session.operation` | `code_exchange`, `refresh`, `scope_validation`, `session_lookup`, `unknown` |
| `token_exchange.session.status_code` | any `int` |

### tokenexchange.get_valid_access_token

Gets a valid access token for the session, and refreshes it when a refresh token is available.

No attributes.

### tokenexchange.impersonation

RFC 8693 impersonation request in the broker. The `tokenexchange.exchange` span, renamed while it runs. It has the same attributes.

| Attribute | Values |
|---|---|
| `impersonation.target_agent_id` | any `string` |
| `token_exchange.agent.id` | any `string` |
| `token_exchange.exchange_kind` | `impersonation`, `third_party` |
| `token_exchange.failure_detail` | `access_token_expired`, `agent_invalid`, `agent_missing`, `agent_repository_unavailable`, `caller_canceled`, `cel_configuration`, `cel_evaluation_failed`, `client_expired`, `client_invalid`, `client_policy_denied`, `credential_identity_changed`, `credential_source_unavailable`, `grant_empty`, `grant_expired`, `grant_missing`, `grant_permission_set_missing`, `grant_permission_set_undeclared`, `grant_repository_unavailable`, `grant_scope_intersection_empty`, `grant_service_definition_missing`, `grant_service_omitted`, `grant_service_requirement_excluded`, `internal_unclassified`, `jwks_unavailable`, `jwt_configuration`, `none`, `provider_client_rejected`, `provider_rejected`, `provider_response_invalid`, `provider_unavailable`, `refresh_rejected`, `refresh_unavailable`, `request_malformed`, `resource_ambiguous`, `resource_missing`, `resource_repository_unavailable`, `resource_unregistered`, `response_write_failed`, `session_configuration`, `session_decryption_failed`, `session_encryption_failed`, `session_missing`, `session_persistence_failed`, `session_repository_unavailable`, `session_scope_insufficient`, `subject_category_ineligible`, `subject_category_unsupported`, `subject_expired`, `subject_invalid` |
| `token_exchange.failure_stage` | [13 values](#token_exchangefailure_stage) |
| `token_exchange.grant.id` | any `string` |
| `token_exchange.grant.updated_at` | any `string` |
| `token_exchange.grant.valid_until` | any `string` |
| `token_exchange.outcome` | `authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success` |
| `token_exchange.recovery_action` | `fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry` |
| `token_exchange.recovery_target` | `broker_configuration`, `calling_client`, `consent`, `none`, `provider_session`, `subject_identity` |
| `token_exchange.service.id` | any `string` |
| `token_exchange.session.dependency` | `credential_source`, `encryption`, `none`, `provider`, `provider_repository`, `session_repository`, `signing` |
| `token_exchange.session.detail` | [16 values](#token_exchangesessiondetail) |
| `token_exchange.session.kind` | `canceled`, `configuration`, `infrastructure`, `internal`, `provider`, `session` |
| `token_exchange.session.oauth_code` | any `string` |
| `token_exchange.session.operation` | `code_exchange`, `refresh`, `scope_validation`, `session_lookup`, `unknown` |
| `token_exchange.session.status_code` | any `int` |

### tokenexchange.verify_agent_access

Checks that the principal has an active grant for the agent.

No attributes.

## Metric attributes

### approvals_pending_total

Number of pending tool approvals.

| Attribute | Values |
|---|---|
| `agent_id` | any `string` |

### extproc.approval.gate.decisions

Approval gate decisions in ExtProc.

| Attribute | Values |
|---|---|
| `outcome` | `deny`, `elicit`, `proceed` |

### extproc.approval.sync_age

Seconds since the last successful approval sync.

No attributes.

### extproc.token_exchange.duration

Duration of token exchange requests in seconds.

| Attribute | Values |
|---|---|
| `token_exchange.exchange_kind` | `impersonation`, `third_party`, `unknown` |
| `token_exchange.failure_detail` | `authorization_denied`, `broker_response_invalid`, `broker_unavailable`, `caller_canceled`, `circuit_open`, `client_expired`, `client_invalid`, `configuration`, `consent_required`, `credential_rejected`, `internal_unclassified`, `none`, `policy_configuration`, `policy_denied`, `policy_evaluation_failed`, `provider_response_invalid`, `provider_unavailable`, `reauth_required`, `request_malformed`, `resource_missing`, `response_write_failed`, `session_reauth_required`, `subject_invalid` |
| `token_exchange.failure_stage` | `client_authorization`, `exchange_routing`, `none`, `refresh`, `request_validation`, `response_write` |
| `token_exchange.outcome` | `authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success` |
| `token_exchange.recovery_action` | `fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry` |
| `token_exchange.recovery_target` | `consent`, `none`, `provider_session` |

### extproc.token_exchange.requests

Total number of token exchange requests processed by ExtProc.

| Attribute | Values |
|---|---|
| `token_exchange.exchange_kind` | `impersonation`, `third_party`, `unknown` |
| `token_exchange.failure_detail` | `authorization_denied`, `broker_response_invalid`, `broker_unavailable`, `caller_canceled`, `circuit_open`, `client_expired`, `client_invalid`, `configuration`, `consent_required`, `credential_rejected`, `internal_unclassified`, `none`, `policy_configuration`, `policy_denied`, `policy_evaluation_failed`, `provider_response_invalid`, `provider_unavailable`, `reauth_required`, `request_malformed`, `resource_missing`, `response_write_failed`, `session_reauth_required`, `subject_invalid` |
| `token_exchange.failure_stage` | `client_authorization`, `exchange_routing`, `none`, `refresh`, `request_validation`, `response_write` |
| `token_exchange.outcome` | `authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success` |
| `token_exchange.recovery_action` | `fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry` |
| `token_exchange.recovery_target` | `consent`, `none`, `provider_session` |

### long_poll_connections_active

Number of active long-poll connections.

No attributes.

### oauth2_sessions

Stored third-party sessions by service, access-token expiry class and refreshability. Recorded in each export cycle with a successful health query: one point per service with sessions and per combination of `expiry` and `refreshable`.

| Attribute | Values |
|---|---|
| `expiry` | `expired`, `expiring`, `unknown`, `valid` |
| `refreshable` | any `string` |
| `service_id` | any `string` |

### proactive_refresh_dropped_total

Background session refreshes dropped because the pool is full. Incremented when a background refresh is rejected because every slot is busy.

| Attribute | Values |
|---|---|
| `triggered_by` | `background` |

### proactive_refresh_failed_total

Admitted background session refreshes that failed. Incremented when an accepted background refresh ends in an error. A deleted session and a refresh that is not due do not count.

| Attribute | Values |
|---|---|
| `failure_detail` | [16 values](#failure_detail) |
| `service_id` | any `string` |
| `triggered_by` | `background` |

### proactive_refresh_triggered_total

Background session refreshes admitted to the pool. Incremented when a background refresh is accepted into the pool.

| Attribute | Values |
|---|---|
| `service_id` | any `string` |
| `triggered_by` | `background` |

### session_sweep_duration_seconds

Duration of admin session sweep requests by outcome. Recorded for each sweep request with an operator principal.

| Attribute | Values |
|---|---|
| `dry_run` | any `string` |
| `outcome` | `aborted`, `canceled`, `completed`, `rejected` |

### session_sweep_failed_total

Admin sweep candidates that could not be refreshed. Incremented when a sweep that is not a dry run classifies a candidate as failed.

| Attribute | Values |
|---|---|
| `failure_detail` | [16 values](#failure_detail) |
| `service_id` | any `string` |
| `triggered_by` | `sweep` |

### session_sweep_refreshed_total

Sessions refreshed and committed by an admin sweep. Incremented when a sweep that is not a dry run commits a refreshed session.

| Attribute | Values |
|---|---|
| `service_id` | any `string` |
| `triggered_by` | `sweep` |

### session_sweep_skipped_total

Admin sweep candidates skipped because they were already refreshed or deleted. Incremented for each skipped candidate in a sweep that is not a dry run.

| Attribute | Values |
|---|---|
| `triggered_by` | `sweep` |

## Attributes

| Attribute | Type | Description |
|---|---|---|
| `agent_id` | `string` | Agent ID. |
| `approval.agent_id` | `string` | Approval agent ID. |
| `approval.count` | `int` | Approval count. |
| `approval.id` | `string` | Approval ID. |
| `approval.is_new` | `boolean` | Approval is new. |
| `approval.outcome` | [enumerated](#approvaloutcome) | Approval outcome. |
| `approval.pair_count` | `int` | Approval pair count. |
| `approval.persistence` | [enumerated](#approvalpersistence) | Persistence scope of the approval decision. |
| `approval.principal` | `string` | Approval principal. |
| `approval.status` | [enumerated](#approvalstatus) | Lifecycle state of the tool approval. |
| `approval.sync_version` | `int` | Approval sync version. |
| `approval.tool_name` | `string` | Approval tool name. |
| `approval.tool_pattern` | `string` | Approval tool pattern. |
| `authorization.action` | `string` | Authorization action. |
| `authorization.duration_ms` | `int` | Authorization duration ms. |
| `authorization.method` | `string` | Authorization method. |
| `authorization.protocol` | `string` | Authorization protocol. |
| `authorization.result_code` | [enumerated](#authorizationresult_code) | Authorization result code. |
| `authorization.target_server_name` | `string` | Authorization target server name. |
| `authorization.tool_name` | `string` | Authorization tool name. |
| `db.operation` | [enumerated](#dboperation) | Repository method name. |
| `db.system` | [enumerated](#dbsystem) | Database system. |
| `dry_run` | `string` | Whether the sweep is a dry run. |
| `encryption.key_type` | [enumerated](#encryptionkey_type) | Encryption key type. |
| `error.type` | [enumerated](#errortype) | Class of the error. Set when the operation fails. |
| `expiry` | [enumerated](#expiry) | Access-token expiry class of the stored session. |
| `failure_detail` | [enumerated](#failure_detail) | Failure detail. |
| `http.request.method` | `string` | HTTP method: an allowlisted method, `_OTHER` otherwise. |
| `http.response.status_code` | `int` | HTTP status code. |
| `impersonation.target_agent_id` | `string` | Impersonation target agent ID. |
| `oauth2.refresh.outcome` | [enumerated](#oauth2refreshoutcome) | Outcome of the background refresh. |
| `oauth2.refresh.triggered_by` | [enumerated](#oauth2refreshtriggered_by) | What triggered the refresh. |
| `outcome` | [enumerated](#outcome) | Outcome. |
| `refreshable` | `string` | Whether the stored session can be refreshed. |
| `service_id` | `string` | Canonical ID of the service when one is set, otherwise its UUID. |
| `storage.outcome` | [enumerated](#storageoutcome) | Result of a storage call that is not a failure. The span status stays unset. |
| `token_exchange.agent.id` | `string` | Token exchange agent ID. |
| `token_exchange.broker_status_code` | `int` | Token exchange broker status code. |
| `token_exchange.dependency` | [enumerated](#token_exchangedependency) | Token exchange dependency. |
| `token_exchange.exchange_kind` | [enumerated](#token_exchangeexchange_kind) | Token exchange exchange kind. |
| `token_exchange.failure_detail` | [enumerated](#token_exchangefailure_detail) | Token exchange failure detail. |
| `token_exchange.failure_stage` | [enumerated](#token_exchangefailure_stage) | Token exchange failure stage. |
| `token_exchange.grant.id` | `string` | Token exchange grant ID. |
| `token_exchange.grant.updated_at` | `string` | Token exchange grant updated at. |
| `token_exchange.grant.valid_until` | `string` | Token exchange grant valid until. |
| `token_exchange.oauth_error_code` | `string` | Token exchange oauth error code. |
| `token_exchange.operation` | [enumerated](#token_exchangeoperation) | ExtProc operation that failed. |
| `token_exchange.outcome` | [enumerated](#token_exchangeoutcome) | Token exchange outcome. |
| `token_exchange.recovery_action` | [enumerated](#token_exchangerecovery_action) | Token exchange recovery action. |
| `token_exchange.recovery_target` | [enumerated](#token_exchangerecovery_target) | Token exchange recovery target. |
| `token_exchange.service.id` | `string` | Registered ID of the service. |
| `token_exchange.session.dependency` | [enumerated](#token_exchangesessiondependency) | Token exchange session dependency. |
| `token_exchange.session.detail` | [enumerated](#token_exchangesessiondetail) | Token exchange session detail. |
| `token_exchange.session.kind` | [enumerated](#token_exchangesessionkind) | Token exchange session kind. |
| `token_exchange.session.oauth_code` | `string` | Token exchange session oauth code. |
| `token_exchange.session.operation` | [enumerated](#token_exchangesessionoperation) | Token exchange session operation. |
| `token_exchange.session.status_code` | `int` | Token exchange session status code. |
| `triggered_by` | [enumerated](#triggered_by) | What triggered the refresh. |

## Attribute values

Each enumerated attribute, with every value that a span or a metric sets.

### approval.outcome

`deny`, `elicit`, `proceed`

### approval.persistence

`once`, `permanent`, `session`

### approval.status

`approved`, `denied`, `pending`

### authorization.result_code

`error`, `ok`, `timeout`, `undefined`, `undefined_action`, `unsupported_action`

### db.operation

`ApproveToolApproval`, `AuthorizationCodeRepo.Create`, `AuthorizationCodeRepo.DeleteExpired`, `AuthorizationCodeRepo.FindByCodeHash`, `AuthorizationCodeRepo.MarkUsed`, `ClientAssertionReplayRepo.Consume`, `ClientAssertionReplayRepo.DeleteExpired`, `ClientCredentialRepo.Create`, `ClientCredentialRepo.Delete`, `ClientCredentialRepo.GetByAgentID`, `ClientCredentialRepo.Rotate`, `ConsumeToolApproval`, `CountPendingToolApprovals`, `CreateToolApproval`, `CreateUserGrant`, `DenyToolApproval`, `GetAgent`, `GetApprovalSyncVersion`, `GetToolApproval`, `IncrementApprovalSyncVersion`, `ListAllActiveToolApprovals`, `ListByPrincipal`, `ListPermanentToolApprovals`, `PKCESessionRepo.Create`, `PKCESessionRepo.Delete`, `PKCESessionRepo.DeleteExpired`, `PKCESessionRepo.FindBySignature`, `RefreshTokenSessionRepo.Create`, `RefreshTokenSessionRepo.DeleteExpired`, `RefreshTokenSessionRepo.FindBySignature`, `RefreshTokenSessionRepo.MarkUsed`, `RefreshTokenSessionRepo.RevokeByAgent`, `RefreshTokenSessionRepo.RevokeByPrincipalAndAgent`, `RefreshTokenSessionRepo.RevokeByRequestID`, `RevokePermanentToolApproval`

### db.system

`postgresql`

### encryption.key_type

`aws_kms`

### error.type

| Value | Meaning |
|---|---|
| `assertion_expired` |  |
| `caller_canceled` |  |
| `circuit_open` |  |
| `configuration` |  |
| `conflict` | Operation conflicts with existing data. |
| `connection` | Storage backend connection failed or disconnected. |
| `internal` |  |
| `invalid_response` |  |
| `jwks_unavailable` |  |
| `network_error` |  |
| `none` |  |
| `not_found` | Requested entity does not exist. |
| `rejected` |  |
| `response_close_failed` |  |
| `response_read_failed` |  |
| `timeout` | Operation exceeded its timeout deadline. |
| `unavailable` |  |
| `unknown` | Error cause is unknown or unexpected. |
| `validation` | Input validation failed. |

### expiry

| Value | Meaning |
|---|---|
| `expired` | The access token has already expired. |
| `expiring` | The access token expires within the refresh lookahead. |
| `unknown` | No access-token expiry is recorded. |
| `valid` | The access token expires after now plus the refresh lookahead. |

### failure_detail

`access_token_expired`, `caller_canceled`, `configuration`, `credential_source_unavailable`, `decryption_failed`, `encryption_failed`, `internal_unclassified`, `persistence_failed`, `provider_client_rejected`, `provider_rejected`, `provider_response_invalid`, `provider_unavailable`, `refresh_rejected`, `refresh_unavailable`, `repository_unavailable`, `session_missing`

### oauth2.refresh.outcome

`failed`, `not_due`, `refreshed`

### oauth2.refresh.triggered_by

`background`

### outcome

`aborted`, `canceled`, `completed`, `deny`, `elicit`, `proceed`, `rejected`

### storage.outcome

| Value | Meaning |
|---|---|
| `canceled` | The caller canceled the operation. |
| `conflict` | Operation conflicts with existing data. |
| `connection` | Storage backend connection failed or disconnected. |
| `not_found` | Requested entity does not exist. |
| `timeout` | Operation exceeded its timeout deadline. |
| `unknown` | Error cause is unknown or unexpected. |
| `validation` | Input validation failed. |

### token_exchange.dependency

`broker`, `local`, `oauth_provider`

### token_exchange.exchange_kind

`impersonation`, `third_party`, `unknown`

### token_exchange.failure_detail

`access_token_expired`, `agent_invalid`, `agent_missing`, `agent_repository_unavailable`, `authorization_denied`, `broker_response_invalid`, `broker_unavailable`, `caller_canceled`, `cel_configuration`, `cel_evaluation_failed`, `circuit_open`, `client_expired`, `client_invalid`, `client_policy_denied`, `configuration`, `consent_required`, `credential_identity_changed`, `credential_rejected`, `credential_source_unavailable`, `grant_empty`, `grant_expired`, `grant_missing`, `grant_permission_set_missing`, `grant_permission_set_undeclared`, `grant_repository_unavailable`, `grant_scope_intersection_empty`, `grant_service_definition_missing`, `grant_service_omitted`, `grant_service_requirement_excluded`, `internal_unclassified`, `jwks_unavailable`, `jwt_configuration`, `none`, `policy_configuration`, `policy_denied`, `policy_evaluation_failed`, `provider_client_rejected`, `provider_rejected`, `provider_response_invalid`, `provider_unavailable`, `reauth_required`, `refresh_rejected`, `refresh_unavailable`, `request_malformed`, `resource_ambiguous`, `resource_missing`, `resource_repository_unavailable`, `resource_unregistered`, `response_write_failed`, `session_configuration`, `session_decryption_failed`, `session_encryption_failed`, `session_missing`, `session_persistence_failed`, `session_reauth_required`, `session_repository_unavailable`, `session_scope_insufficient`, `subject_category_ineligible`, `subject_category_unsupported`, `subject_expired`, `subject_invalid`

### token_exchange.failure_stage

`client_authorization`, `client_validation`, `exchange_routing`, `grant_authorization`, `identity_resolution`, `none`, `refresh`, `request_validation`, `resource_resolution`, `response_write`, `scope_validation`, `session_lookup`, `subject_validation`

### token_exchange.operation

`assertion_refresh`, `configuration`, `exchange`

### token_exchange.outcome

`authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success`

### token_exchange.recovery_action

`fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry`

### token_exchange.recovery_target

`broker_configuration`, `calling_client`, `consent`, `none`, `provider_session`, `subject_identity`

### token_exchange.session.dependency

`credential_source`, `encryption`, `none`, `provider`, `provider_repository`, `session_repository`, `signing`

### token_exchange.session.detail

`access_token_expired`, `caller_canceled`, `configuration`, `credential_source_unavailable`, `decryption_failed`, `encryption_failed`, `internal_unclassified`, `persistence_failed`, `provider_client_rejected`, `provider_rejected`, `provider_response_invalid`, `provider_unavailable`, `refresh_rejected`, `refresh_unavailable`, `repository_unavailable`, `session_missing`

### token_exchange.session.kind

`canceled`, `configuration`, `infrastructure`, `internal`, `provider`, `session`

### token_exchange.session.operation

`code_exchange`, `refresh`, `scope_validation`, `session_lookup`, `unknown`

### triggered_by

`background`, `sweep`
