---
title: "Telemetry"
description: Spans, metrics and attributes that the broker and ExtProc emit through OpenTelemetry. Generated from the source code.
---

# Telemetry

This page lists every span, metric and attribute that the broker and ExtProc create through
the OpenTelemetry API. `tools/telemetrydoc` generates it from the source code, and a test
fails when the page is out of date. Do not edit it by hand. The span and attribute
descriptions are in `docs/reference/telemetry-descriptions.yaml`. After you change a span,
a metric or an attribute, run `just telemetry-doc`.

The page does not list:

- the HTTP server spans and metrics that `otelchi` creates for each inbound request;
- metrics that the OpenTelemetry SDK, the Go runtime or a library creates;
- span events and resource attributes;
- log records. `coding-standards/LOGGING.md` covers them.

A value list can show more values than one span or metric sets. An attribute with an
enumerated Go type lists every constant of the type. A variable or a parameter lists every
constant that the code can assign or pass to it. An attribute that the code sets on the span
in a context is listed under each span of this module that the context can hold. Rules for
adding telemetry are in `coding-standards/OPENTELEMETRY.md`.

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
| [`approval.revoke`](#approvalrevoke) | Internal | `approval` | No | Revokes a permanent approval or a permanent denial. It becomes denied, with no persistence. |
| [`approval.sync`](#approvalsync) | Internal | `approval` | No | Returns the sync version and the active approvals, grouped by principal and agent. |
| [`encryption.decrypt`](#encryptiondecrypt) | Internal | `encryption` | No | Decrypts data with the AWS Encryption SDK, through the AWS KMS hierarchical keyring or, with a base64 key, a raw AES keyring. |
| [`encryption.encrypt`](#encryptionencrypt) | Internal | `encryption` | No | Encrypts data with the AWS Encryption SDK, through the AWS KMS hierarchical keyring or, with a base64 key, a raw AES keyring. |
| [`extproc.approval.gate`](#extprocapprovalgate) | Internal | `extproc` | No | Decides whether a tool call proceeds, needs the user's approval first, or is denied. It does not wait for the approval. |
| [`extproc.client_assertion.refresh`](#extprocclient_assertionrefresh) | Internal | `extproc` | Yes | Obtains a new client assertion through the client credentials grant. |
| [`extproc.grant_context.evaluate`](#extprocgrant_contextevaluate) | Client | `extproc` | Yes | Calls the broker's AuthZEN access evaluation endpoint for the subject token and the requested resource. The resolver calls it on a cache miss, once for all concurrent requests for the same token and resource. |
| [`extproc.grant_context.resolve`](#extprocgrant_contextresolve) | Internal | `extproc` | Yes | Returns the delegation permit for the subject token and the requested resource, from ExtProc's permit cache or from the broker. With `grant_context.enabled`, ExtProc resolves it before each OPA decision. |
| [`extproc.opa.evaluate`](#extprocopaevaluate) | Internal | `extproc` | No | Evaluates the OPA policy for a request. |
| [`extproc.token_exchange`](#extproctoken_exchange) | Server | `extproc` | Yes | Handles the request headers of one gateway request in ExtProc. It validates the request metadata and gets an exchanged token from ExtProc's cache or from the broker. For a request with a body, the OPA decision on the body comes after the span ends. |
| [`http.request`](#httprequest) | Client | `telemetryhttp` | Yes | Outbound HTTP call through `telemetryhttp`. The span does not record URLs, headers, bodies or error messages. |
| [`jwks.fetch`](#jwksfetch) | Internal | `jwks` | Yes | Returns the JSON Web Key Set that the URL or file adapter holds, and fails when the set is stale. The URL adapter fetches the set on the first call. |
| [`oauth2.token_proxy`](#oauth2token_proxy) | Internal | `upstream` | No | Forwards a token request to the upstream authorization server and returns its response. |
| [`oauth2session.background_refresh`](#oauth2sessionbackground_refresh) | Internal | `oauth2session` | No | Third-party session refresh in the background. It is a new root span, with a link to the span of the request that triggered the refresh. |
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
| [`storage.increment_version.approval_sync`](#storageincrement_versionapproval_sync) | Client | `storage` | Yes | Increments the approval sync version and issues `NOTIFY`. With the PostgreSQL approval repository, a trigger increments the version and this span is absent. |
| [`storage.list.user_grants_by_principal`](#storagelistuser_grants_by_principal) | Client | `storage` | Yes | Retrieves the active grants of a principal across all agents. |
| [`storage.list_active.tool_approval`](#storagelist_activetool_approval) | Client | `storage` | Yes | Lists the approvals and denials that are not consumed. A pending approval must not be expired, and a session-scoped one must belong to an active agent session. |
| [`storage.list_permanent.tool_approval`](#storagelist_permanenttool_approval) | Client | `storage` | Yes | Lists the permanent approvals and denials of a principal. |
| [`storage.mark_used.authorization_code`](#storagemark_usedauthorization_code) | Client | `storage` | Yes | Marks an unused authorization code as used. |
| [`storage.mark_used.refresh_token_session`](#storagemark_usedrefresh_token_session) | Client | `storage` | Yes | Marks an unused refresh token session as used. |
| [`storage.revoke_by_agent.refresh_token_session`](#storagerevoke_by_agentrefresh_token_session) | Client | `storage` | Yes | Revokes the unused refresh token sessions of an agent. |
| [`storage.revoke_by_principal_and_agent.refresh_token_session`](#storagerevoke_by_principal_and_agentrefresh_token_session) | Client | `storage` | Yes | Revokes the unused refresh token sessions of a principal and agent pair. |
| [`storage.revoke_by_request.refresh_token_session`](#storagerevoke_by_requestrefresh_token_session) | Client | `storage` | Yes | Revokes the unused refresh token sessions of a token request. |
| [`storage.revoke_permanent.tool_approval`](#storagerevoke_permanenttool_approval) | Client | `storage` | Yes | Revokes a permanent approval or a permanent denial in PostgreSQL. It becomes denied, with no persistence. |
| [`storage.rotate.client_credential`](#storagerotateclient_credential) | Client | `storage` | Yes | Replaces the client credential of an agent in one transaction. |
| [`tokenexchange.access_evaluation`](#tokenexchangeaccess_evaluation) | Internal | `tokenexchange` | Yes | Answers an AuthZEN access evaluation request, such as one from ExtProc's grant context. It checks whether the user's grant lets the agent access the requested resource, and does not issue a token. |
| [`tokenexchange.exchange`](#tokenexchangeexchange) | Internal | `tokenexchange` | Yes | RFC 8693 token exchange request in the broker. |
| [`tokenexchange.get_valid_access_token`](#tokenexchangeget_valid_access_token) | Internal | `tokenexchange` | No | Returns the access token of the session. Refreshes it during the request when it has expired, and starts a background refresh when it expires within the refresh lookahead. |
| [`tokenexchange.impersonation`](#tokenexchangeimpersonation) | Internal | `tokenexchange` | Yes | RFC 8693 impersonation request in the broker. |
| [`tokenexchange.verify_agent_access`](#tokenexchangeverify_agent_access) | Internal | `tokenexchange` | Yes | Checks that the principal has an active grant for the agent. |

## Metrics

| Metric | Instrument | Unit | Scope | Description |
|---|---|---|---|---|
| [`approvals_pending_total`](#approvals_pending_total) | Int64UpDownCounter | `{approval}` | `approval` | Number of pending tool approvals |
| [`extproc.approval.gate.decisions`](#extprocapprovalgatedecisions) | Int64Counter |  | `extproc` |  |
| [`extproc.approval.sync_age`](#extprocapprovalsync_age) | Int64ObservableGauge | `s` | `extproc` | Seconds since the last successful approval sync |
| [`extproc.token_exchange.duration`](#extproctoken_exchangeduration) | Float64Histogram | `s` | `extproc` | Duration of token exchange requests in seconds |
| [`extproc.token_exchange.requests`](#extproctoken_exchangerequests) | Int64Counter |  | `extproc` | Total number of token exchange requests processed by ExtProc |
| [`long_poll_connections_active`](#long_poll_connections_active) | Int64UpDownCounter | `{connection}` | `approval` | Number of active long-poll connections |
| [`oauth2_sessions`](#oauth2_sessions) | Int64ObservableGauge | `{session}` | `oauth2session` | Stored third-party sessions by service, access-token expiry class and refreshability |
| [`proactive_refresh_dropped_total`](#proactive_refresh_dropped_total) | Int64Counter | `{refresh}` | `oauth2session` | Background session refreshes dropped because the pool is full |
| [`proactive_refresh_failed_total`](#proactive_refresh_failed_total) | Int64Counter | `{refresh}` | `oauth2session` | Admitted background session refreshes that failed |
| [`proactive_refresh_triggered_total`](#proactive_refresh_triggered_total) | Int64Counter | `{refresh}` | `oauth2session` | Background session refreshes admitted to the pool |
| [`session_sweep_duration_seconds`](#session_sweep_duration_seconds) | Float64Histogram | `s` | `oauth2session` | Duration of admin session sweep requests by outcome |
| [`session_sweep_failed_total`](#session_sweep_failed_total) | Int64Counter | `{refresh}` | `oauth2session` | Admin sweep candidates that could not be refreshed |
| [`session_sweep_refreshed_total`](#session_sweep_refreshed_total) | Int64Counter | `{refresh}` | `oauth2session` | Sessions refreshed and committed by an admin sweep |
| [`session_sweep_skipped_total`](#session_sweep_skipped_total) | Int64Counter | `{refresh}` | `oauth2session` | Admin sweep candidates skipped because they were already refreshed or deleted |

## Span attributes

### approval.approve

Transitions a pending approval to approved.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.persistence` | [`internal/domain/storage.ApprovalPersistence`](#internaldomainstorageapprovalpersistence) |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |
| `approval.tool_pattern` | any `string` |

### approval.consume

Marks an approved approval with persistence `once` as consumed.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.tool_name` | any `string` |

### approval.created

Creates a pending approval, or returns the existing one.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.is_new` | any `bool` |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |

### approval.deny

Transitions a pending approval to denied.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |

### approval.list_pending

Lists the pending approvals of a principal.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.count` | any `int` |
| `approval.principal` | any `string` |

### approval.list_permanent

Lists the permanent approvals and denials of a principal.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.count` | any `int` |
| `approval.principal` | any `string` |

### approval.review

Retrieves an approval by ID for the approval view.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.principal` | any `string` |
| `approval.status` | [`internal/domain/storage.ApprovalStatus`](#internaldomainstorageapprovalstatus) |
| `approval.tool_name` | any `string` |

### approval.revoke

Revokes a permanent approval or a permanent denial. It becomes denied, with no persistence.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.tool_name` | any `string` |

### approval.sync

Returns the sync version and the active approvals, grouped by principal and agent.

Started in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `approval.pair_count` | any `int` |
| `approval.sync_version` | any `int64` |

### encryption.decrypt

Decrypts data with the AWS Encryption SDK, through the AWS KMS hierarchical keyring or, with a base64 key, a raw AES keyring.

Started in `internal/adapters/encryption/aws/adapter.go`.

| Attribute | Values |
|---|---|
| `encryption.key_type` | `aws_kms` |

### encryption.encrypt

Encrypts data with the AWS Encryption SDK, through the AWS KMS hierarchical keyring or, with a base64 key, a raw AES keyring.

Started in `internal/adapters/encryption/aws/adapter.go`.

| Attribute | Values |
|---|---|
| `encryption.key_type` | `aws_kms` |

### extproc.approval.gate

Decides whether a tool call proceeds, needs the user's approval first, or is denied. It does not wait for the approval.

Started in `internal/extproc/approval/gate.go`.

| Attribute | Values |
|---|---|
| `approval.outcome` | `deny`, `elicit`, `proceed` |

### extproc.client_assertion.refresh

Obtains a new client assertion through the client credentials grant.

Started in `internal/extproc/server/exchanger.go`.

| Attribute | Values |
|---|---|
| `error.type` | [`internal/extproc/server.ErrorKind`](#internalextprocservererrorkind) |
| `http.response.status_code` | any `int` |
| `token_exchange.dependency` | [`internal/extproc/server.Dependency`](#internalextprocserverdependency) |
| `token_exchange.exchange_kind` | [`internal/extproc/server.ExchangeKind`](#internalextprocserverexchangekind) |
| `token_exchange.failure_detail` | [`internal/extproc/server.FailureDetail`](#internalextprocserverfailuredetail) |
| `token_exchange.failure_stage` | [`internal/extproc/server.FailureStage`](#internalextprocserverfailurestage) |
| `token_exchange.oauth_error_code` | any `string` |
| `token_exchange.operation` | [`internal/extproc/server.Operation`](#internalextprocserveroperation) |
| `token_exchange.outcome` | [`internal/extproc/server.Outcome`](#internalextprocserveroutcome) |
| `token_exchange.recovery_action` | [`internal/extproc/server.RecoveryAction`](#internalextprocserverrecoveryaction) |
| `token_exchange.recovery_target` | [`internal/extproc/server.RecoveryTarget`](#internalextprocserverrecoverytarget) |

### extproc.grant_context.evaluate

Calls the broker's AuthZEN access evaluation endpoint for the subject token and the requested resource. The resolver calls it on a cache miss, once for all concurrent requests for the same token and resource.

Started in `internal/extproc/grantcontext/client.go`.

| Attribute | Values |
|---|---|
| `error.type` | `unavailable`, any `string` |
| `token_exchange.failure_detail` | `caller_canceled`, `grant_context_denied`, `grant_context_unavailable`, `none` |
| `token_exchange.operation` | `grant_resolution` |
| `token_exchange.outcome` | `authorization_denied`, `canceled`, `infrastructure_error`, `success` |

### extproc.grant_context.resolve

Returns the delegation permit for the subject token and the requested resource, from ExtProc's permit cache or from the broker. With `grant_context.enabled`, ExtProc resolves it before each OPA decision.

Started in `internal/extproc/grantcontext/resolver.go`.

| Attribute | Values |
|---|---|
| `error.type` | `unavailable`, any `string` |
| `grant_context.cache` | `hit`, `miss` |
| `token_exchange.failure_detail` | `caller_canceled`, `grant_context_denied`, `grant_context_unavailable`, `none` |
| `token_exchange.operation` | `grant_resolution` |
| `token_exchange.outcome` | `authorization_denied`, `canceled`, `infrastructure_error`, `success` |

### extproc.opa.evaluate

Evaluates the OPA policy for a request.

Started in `internal/extproc/authorization/authorizer.go`.

| Attribute | Values |
|---|---|
| `authorization.action` | `deny`, any `string` |
| `authorization.duration_ms` | any `int64` |
| `authorization.method` | any `string` |
| `authorization.protocol` | any `string` |
| `authorization.result_code` | `error`, `ok`, `timeout`, `undefined`, `undefined_action`, `unsupported_action` |
| `authorization.target_server_name` | any `string` |
| `authorization.tool_name` | any `string` |

### extproc.token_exchange

Handles the request headers of one gateway request in ExtProc. It validates the request metadata and gets an exchanged token from ExtProc's cache or from the broker. For a request with a body, the OPA decision on the body comes after the span ends.

Started in `internal/extproc/server/server.go`.

| Attribute | Values |
|---|---|
| `error.type` | [`internal/extproc/server.ErrorKind`](#internalextprocservererrorkind) |
| `token_exchange.broker_status_code` | any `int` |
| `token_exchange.dependency` | [`internal/extproc/server.Dependency`](#internalextprocserverdependency) |
| `token_exchange.exchange_kind` | [`internal/extproc/server.ExchangeKind`](#internalextprocserverexchangekind) |
| `token_exchange.failure_detail` | [`internal/extproc/server.FailureDetail`](#internalextprocserverfailuredetail) |
| `token_exchange.failure_stage` | [`internal/extproc/server.FailureStage`](#internalextprocserverfailurestage) |
| `token_exchange.oauth_error_code` | any `string` |
| `token_exchange.operation` | [`internal/extproc/server.Operation`](#internalextprocserveroperation) |
| `token_exchange.outcome` | [`internal/extproc/server.Outcome`](#internalextprocserveroutcome) |
| `token_exchange.recovery_action` | [`internal/extproc/server.RecoveryAction`](#internalextprocserverrecoveryaction) |
| `token_exchange.recovery_target` | [`internal/extproc/server.RecoveryTarget`](#internalextprocserverrecoverytarget) |

### http.request

Outbound HTTP call through `telemetryhttp`. The span does not record URLs, headers, bodies or error messages.

Started in `internal/telemetryhttp/transport.go`.

| Attribute | Values |
|---|---|
| `error.type` | `network_error`, `response_close_failed`, `response_read_failed`, any `string` |
| `http.request.method` | any `string` |
| `http.response.status_code` | any `int` |

### jwks.fetch

Returns the JSON Web Key Set that the URL or file adapter holds, and fails when the set is stale. The URL adapter fetches the set on the first call.

Started in `internal/adapters/jwks/adapter.go`.

| Attribute | Values |
|---|---|
| `error.type` | `jwks_unavailable` |

### oauth2.token_proxy

Forwards a token request to the upstream authorization server and returns its response.

Started in `internal/adapters/http/enduser/token_grant_strategy.go`.

| Attribute | Values |
|---|---|
| `http.request.method` | `POST` |
| `http.response.status_code` | any `int` |

### oauth2session.background_refresh

Third-party session refresh in the background. It is a new root span, with a link to the span of the request that triggered the refresh.

Started in `internal/domain/oauth2session/background_refresh.go`.

| Attribute | Values |
|---|---|
| `oauth2.refresh.outcome` | `failed`, `not_due`, `refreshed` |
| `oauth2.refresh.triggered_by` | `background` |

### storage.approve.tool_approval

Transitions a pending tool approval to approved in PostgreSQL.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ApproveToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.consume.client_assertion_replay

Records the `jti` of a client assertion, or reports a conflict when it is already recorded.

Started in `internal/adapters/storage/postgres/client_assertion_replay_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientAssertionReplayRepo.Consume` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.consume.tool_approval

Marks an approved tool approval with persistence `once` as consumed in PostgreSQL.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ConsumeToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.count_pending.tool_approval

Counts the pending approvals of a principal and agent pair that are not expired.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `CountPendingToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.authorization_code

Inserts an authorization code.

Started in `internal/adapters/storage/postgres/authorization_code_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.client_credential

Inserts the client credential of an agent.

Started in `internal/adapters/storage/postgres/client_credential_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.pkce_session

Inserts a PKCE session.

Started in `internal/adapters/storage/postgres/pkce_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.refresh_token_session

Inserts a refresh token session.

Started in `internal/adapters/storage/postgres/refresh_token_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.tool_approval

Inserts a tool approval, or returns the existing pending duplicate.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `CreateToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.user_grant

Creates a user grant, or updates the grant of the same principal and agent.

Started in `internal/adapters/storage/postgres/user_grants.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `CreateUserGrant` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete.client_credential

Deletes the client credential of an agent.

Started in `internal/adapters/storage/postgres/client_credential_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Delete` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete.pkce_session

Deletes a PKCE session by its signature.

Started in `internal/adapters/storage/postgres/pkce_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.Delete` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete_expired.authorization_code

Deletes the expired authorization codes.

Started in `internal/adapters/storage/postgres/authorization_code_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete_expired.client_assertion_replay

Deletes the expired client assertion replay records.

Started in `internal/adapters/storage/postgres/client_assertion_replay_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientAssertionReplayRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete_expired.pkce_session

Deletes the expired PKCE sessions.

Started in `internal/adapters/storage/postgres/pkce_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete_expired.refresh_token_session

Deletes the expired refresh token sessions.

Started in `internal/adapters/storage/postgres/refresh_token_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.deny.tool_approval

Transitions a pending tool approval to denied in PostgreSQL.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `DenyToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.find.authorization_code

Retrieves an authorization code that is not expired by its code hash.

Started in `internal/adapters/storage/postgres/authorization_code_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.FindByCodeHash` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.find.pkce_session

Retrieves a PKCE session by its signature.

Started in `internal/adapters/storage/postgres/pkce_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.FindBySignature` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.find.refresh_token_session

Retrieves a refresh token session by its signature.

Started in `internal/adapters/storage/postgres/refresh_token_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.FindBySignature` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.get.agent

Retrieves an agent by ID from PostgreSQL.

Started in `internal/adapters/storage/postgres/agent_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `GetAgent` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.get.client_credential

Retrieves the client credential of an agent.

Started in `internal/adapters/storage/postgres/client_credential_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.GetByAgentID` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.get.tool_approval

Retrieves a tool approval by ID from PostgreSQL.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `GetToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.get_version.approval_sync

Retrieves the current approval sync version.

Started in `internal/adapters/storage/postgres/approval_sync_state_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `GetApprovalSyncVersion` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.increment_version.approval_sync

Increments the approval sync version and issues `NOTIFY`. With the PostgreSQL approval repository, a trigger increments the version and this span is absent.

Started in `internal/adapters/storage/postgres/approval_sync_state_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `IncrementApprovalSyncVersion` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.list.user_grants_by_principal

Retrieves the active grants of a principal across all agents.

Started in `internal/adapters/storage/postgres/user_grants.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ListByPrincipal` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.list_active.tool_approval

Lists the approvals and denials that are not consumed. A pending approval must not be expired, and a session-scoped one must belong to an active agent session.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ListAllActiveToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.list_permanent.tool_approval

Lists the permanent approvals and denials of a principal.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ListPermanentToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.mark_used.authorization_code

Marks an unused authorization code as used.

Started in `internal/adapters/storage/postgres/authorization_code_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.MarkUsed` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.mark_used.refresh_token_session

Marks an unused refresh token session as used.

Started in `internal/adapters/storage/postgres/refresh_token_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.MarkUsed` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.revoke_by_agent.refresh_token_session

Revokes the unused refresh token sessions of an agent.

Started in `internal/adapters/storage/postgres/refresh_token_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByAgent` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.revoke_by_principal_and_agent.refresh_token_session

Revokes the unused refresh token sessions of a principal and agent pair.

Started in `internal/adapters/storage/postgres/refresh_token_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByPrincipalAndAgent` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.revoke_by_request.refresh_token_session

Revokes the unused refresh token sessions of a token request.

Started in `internal/adapters/storage/postgres/refresh_token_session_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByRequestID` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.revoke_permanent.tool_approval

Revokes a permanent approval or a permanent denial in PostgreSQL. It becomes denied, with no persistence.

Started in `internal/adapters/storage/postgres/tool_approval_repository.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `RevokePermanentToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.rotate.client_credential

Replaces the client credential of an agent in one transaction.

Started in `internal/adapters/storage/postgres/client_credential_repo.go`.

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Rotate` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### tokenexchange.access_evaluation

Answers an AuthZEN access evaluation request, such as one from ExtProc's grant context. It checks whether the user's grant lets the agent access the requested resource, and does not issue a token.

Started in `internal/adapters/http/enduser/authzen_evaluation.go`.

| Attribute | Values |
|---|---|
| `access_evaluation.decision` | `deny`, `error`, `permit` |
| `access_evaluation.max_age` | any `int` |
| `http.response.status_code` | any `int` |
| `token_exchange.agent.id` | any `string` |
| `token_exchange.failure_detail` | [`internal/domain/tokenexchange.FailureDetail`](#internaldomaintokenexchangefailuredetail) |
| `token_exchange.failure_stage` | [`internal/domain/tokenexchange.FailureStage`](#internaldomaintokenexchangefailurestage) |
| `token_exchange.grant.id` | any `string` |
| `token_exchange.grant.updated_at` | any `string` |
| `token_exchange.operation` | `access_evaluation` |
| `token_exchange.outcome` | [`internal/domain/tokenexchange.Outcome`](#internaldomaintokenexchangeoutcome) |
| `token_exchange.recovery_action` | [`internal/domain/tokenexchange.RecoveryAction`](#internaldomaintokenexchangerecoveryaction) |
| `token_exchange.recovery_target` | [`internal/domain/tokenexchange.RecoveryTarget`](#internaldomaintokenexchangerecoverytarget) |
| `token_exchange.service.id` | any `string` |

### tokenexchange.exchange

RFC 8693 token exchange request in the broker.

Started in `internal/adapters/http/enduser/oauth2_token.go`.

| Attribute | Values |
|---|---|
| `impersonation.target_agent_id` | any `string` |
| `token_exchange.agent.id` | any `string` |
| `token_exchange.exchange_kind` | [`internal/domain/tokenexchange.ExchangeKind`](#internaldomaintokenexchangeexchangekind) |
| `token_exchange.failure_detail` | [`internal/domain/tokenexchange.FailureDetail`](#internaldomaintokenexchangefailuredetail) |
| `token_exchange.failure_stage` | [`internal/domain/tokenexchange.FailureStage`](#internaldomaintokenexchangefailurestage) |
| `token_exchange.grant.id` | any `string` |
| `token_exchange.grant.updated_at` | any `string` |
| `token_exchange.grant.valid_until` | any `string` |
| `token_exchange.outcome` | [`internal/domain/tokenexchange.Outcome`](#internaldomaintokenexchangeoutcome) |
| `token_exchange.recovery_action` | [`internal/domain/tokenexchange.RecoveryAction`](#internaldomaintokenexchangerecoveryaction) |
| `token_exchange.recovery_target` | [`internal/domain/tokenexchange.RecoveryTarget`](#internaldomaintokenexchangerecoverytarget) |
| `token_exchange.service.id` | any `string` |
| `token_exchange.session.dependency` | [`internal/domain/oauth2session.Dependency`](#internaldomainoauth2sessiondependency) |
| `token_exchange.session.detail` | [`internal/domain/oauth2session.ErrorDetail`](#internaldomainoauth2sessionerrordetail) |
| `token_exchange.session.kind` | [`internal/domain/oauth2session.ErrorKind`](#internaldomainoauth2sessionerrorkind) |
| `token_exchange.session.oauth_code` | any `string` |
| `token_exchange.session.operation` | [`internal/domain/oauth2session.Operation`](#internaldomainoauth2sessionoperation) |
| `token_exchange.session.status_code` | any `int` |

### tokenexchange.get_valid_access_token

Returns the access token of the session. Refreshes it during the request when it has expired, and starts a background refresh when it expires within the refresh lookahead.

Started in `internal/domain/tokenexchange/service.go`.

No attributes.

### tokenexchange.impersonation

RFC 8693 impersonation request in the broker.

The `tokenexchange.exchange` span, renamed while it runs. It has the same attributes.

| Attribute | Values |
|---|---|
| `impersonation.target_agent_id` | any `string` |
| `token_exchange.agent.id` | any `string` |
| `token_exchange.exchange_kind` | [`internal/domain/tokenexchange.ExchangeKind`](#internaldomaintokenexchangeexchangekind) |
| `token_exchange.failure_detail` | [`internal/domain/tokenexchange.FailureDetail`](#internaldomaintokenexchangefailuredetail) |
| `token_exchange.failure_stage` | [`internal/domain/tokenexchange.FailureStage`](#internaldomaintokenexchangefailurestage) |
| `token_exchange.grant.id` | any `string` |
| `token_exchange.grant.updated_at` | any `string` |
| `token_exchange.grant.valid_until` | any `string` |
| `token_exchange.outcome` | [`internal/domain/tokenexchange.Outcome`](#internaldomaintokenexchangeoutcome) |
| `token_exchange.recovery_action` | [`internal/domain/tokenexchange.RecoveryAction`](#internaldomaintokenexchangerecoveryaction) |
| `token_exchange.recovery_target` | [`internal/domain/tokenexchange.RecoveryTarget`](#internaldomaintokenexchangerecoverytarget) |
| `token_exchange.service.id` | any `string` |
| `token_exchange.session.dependency` | [`internal/domain/oauth2session.Dependency`](#internaldomainoauth2sessiondependency) |
| `token_exchange.session.detail` | [`internal/domain/oauth2session.ErrorDetail`](#internaldomainoauth2sessionerrordetail) |
| `token_exchange.session.kind` | [`internal/domain/oauth2session.ErrorKind`](#internaldomainoauth2sessionerrorkind) |
| `token_exchange.session.oauth_code` | any `string` |
| `token_exchange.session.operation` | [`internal/domain/oauth2session.Operation`](#internaldomainoauth2sessionoperation) |
| `token_exchange.session.status_code` | any `int` |

### tokenexchange.verify_agent_access

Checks that the principal has an active grant for the agent.

Started in `internal/domain/tokenexchange/grant_resolver.go`.

| Attribute | Values |
|---|---|
| `error.type` | `grant_repository_unavailable` |

## Metric attributes

### approvals_pending_total

Created in `internal/domain/approval/service.go`.

| Attribute | Values |
|---|---|
| `agent_id` | any `string` |

### extproc.approval.gate.decisions

Created in `internal/extproc/approval/gate.go`.

| Attribute | Values |
|---|---|
| `outcome` | `deny`, `elicit`, `proceed` |

### extproc.approval.sync_age

Created in `internal/extproc/approval/syncer.go`.

No attributes.

### extproc.token_exchange.duration

Created in `internal/extproc/server/server.go`.

| Attribute | Values |
|---|---|
| `token_exchange.exchange_kind` | [`internal/extproc/server.ExchangeKind`](#internalextprocserverexchangekind) |
| `token_exchange.failure_detail` | [`internal/extproc/server.FailureDetail`](#internalextprocserverfailuredetail) |
| `token_exchange.failure_stage` | [`internal/extproc/server.FailureStage`](#internalextprocserverfailurestage) |
| `token_exchange.outcome` | [`internal/extproc/server.Outcome`](#internalextprocserveroutcome) |
| `token_exchange.recovery_action` | [`internal/extproc/server.RecoveryAction`](#internalextprocserverrecoveryaction) |
| `token_exchange.recovery_target` | [`internal/extproc/server.RecoveryTarget`](#internalextprocserverrecoverytarget) |

### extproc.token_exchange.requests

Created in `internal/extproc/server/server.go`.

| Attribute | Values |
|---|---|
| `token_exchange.exchange_kind` | [`internal/extproc/server.ExchangeKind`](#internalextprocserverexchangekind) |
| `token_exchange.failure_detail` | [`internal/extproc/server.FailureDetail`](#internalextprocserverfailuredetail) |
| `token_exchange.failure_stage` | [`internal/extproc/server.FailureStage`](#internalextprocserverfailurestage) |
| `token_exchange.outcome` | [`internal/extproc/server.Outcome`](#internalextprocserveroutcome) |
| `token_exchange.recovery_action` | [`internal/extproc/server.RecoveryAction`](#internalextprocserverrecoveryaction) |
| `token_exchange.recovery_target` | [`internal/extproc/server.RecoveryTarget`](#internalextprocserverrecoverytarget) |

### long_poll_connections_active

Created in `internal/adapters/http/handlers/approval/sync_handler.go`.

No attributes.

### oauth2_sessions

Created in `internal/domain/oauth2session/health_metrics.go`.

| Attribute | Values |
|---|---|
| `expiry` | [`internal/domain/oauth2session.SessionHealthExpiry`](#internaldomainoauth2sessionsessionhealthexpiry) |
| `refreshable` | any `string` |
| `service_id` | any `string` |

### proactive_refresh_dropped_total

Created in `internal/domain/oauth2session/background_refresh.go`.

| Attribute | Values |
|---|---|
| `triggered_by` | `background` |

### proactive_refresh_failed_total

Created in `internal/domain/oauth2session/background_refresh.go`.

| Attribute | Values |
|---|---|
| `failure_detail` | [`internal/domain/oauth2session.ErrorDetail`](#internaldomainoauth2sessionerrordetail) |
| `service_id` | any `string` |
| `triggered_by` | `background` |

### proactive_refresh_triggered_total

Created in `internal/domain/oauth2session/background_refresh.go`.

| Attribute | Values |
|---|---|
| `service_id` | any `string` |
| `triggered_by` | `background` |

### session_sweep_duration_seconds

Created in `internal/adapters/http/handlers/admin/session_sweep_handler.go`.

| Attribute | Values |
|---|---|
| `dry_run` | any `string` |
| `outcome` | `aborted`, `canceled`, `completed`, `rejected` |

### session_sweep_failed_total

Created in `internal/domain/oauth2session/sweep.go`.

| Attribute | Values |
|---|---|
| `failure_detail` | [`internal/domain/oauth2session.ErrorDetail`](#internaldomainoauth2sessionerrordetail) |
| `service_id` | any `string` |
| `triggered_by` | `sweep` |

### session_sweep_refreshed_total

Created in `internal/domain/oauth2session/sweep.go`.

| Attribute | Values |
|---|---|
| `service_id` | any `string` |
| `triggered_by` | `sweep` |

### session_sweep_skipped_total

Created in `internal/domain/oauth2session/sweep.go`.

| Attribute | Values |
|---|---|
| `triggered_by` | `sweep` |

## Attributes

| Attribute | Description |
|---|---|
| `access_evaluation.decision` | Decision that the broker returned. `deny` is a consent problem, such as a missing or expired grant. `error` is a failed request, with the status in `http.response.status_code`. |
| `access_evaluation.max_age` | Number of seconds for which the caller can reuse the permit, as in the `Cache-Control` response header. Set only on a permit. The broker caps it at 60 seconds and at the grant expiry. |
| `agent_id` | ID of the agent that requested the approvals. |
| `approval.agent_id` | ID of the agent that asked to call the tool. |
| `approval.count` | Number of approvals in the list that the call returns. |
| `approval.id` | ID of the tool approval. |
| `approval.is_new` | Whether the call created the approval. It is `false` when the call returned an existing approval for the same request. |
| `approval.outcome` | Decision of the approval gate: the call proceeds, needs the user's approval first (`elicit`), or is denied. `deny` also covers a failure of the gate or the broker. |
| `approval.pair_count` | Number of principal and agent pairs in the sync state. |
| `approval.persistence` | Persistence scope of the approval decision. |
| `approval.principal` | Principal that the approval belongs to. |
| `approval.status` | Lifecycle state of the tool approval. |
| `approval.sync_version` | Version of the approval sync state. A change to an approval increments it. |
| `approval.tool_name` | Name of the tool that the agent asked to call. |
| `approval.tool_pattern` | Tool pattern that the approval covers. |
| `authorization.action` | Action of the policy decision, such as `allow`, `deny` or `approval_required`. When `authorization.result_code` is `unsupported_action`, it is the action that the policy returned, and ExtProc denies the request. |
| `authorization.duration_ms` | Duration of the policy evaluation in milliseconds. |
| `authorization.method` | MCP method of the request, such as `tools/call`. A method that is not on the allowlist is `unknown`. |
| `authorization.protocol` | Protocol of the request: `mcp`, or `unknown`. |
| `authorization.result_code` | How the policy evaluation ended: `ok` when the policy returned a supported action, otherwise the reason why it did not. |
| `authorization.target_server_name` | Name of the MCP server that the request targets. |
| `authorization.tool_name` | Name of the MCP tool that the request calls. |
| `db.operation` | Name of the storage operation, such as `GetAgent`. |
| `db.system` | Database that the storage call uses. |
| `dry_run` | Whether the sweep is a dry run. |
| `encryption.key_type` | Always `aws_kms`, also with a raw AES key. |
| `error.type` | Class of the error. Set when the operation fails. |
| `expiry` | Access-token expiry class of the stored session. |
| `failure_detail` | Detail code of the failed refresh, such as `refresh_rejected`. The `session.oauth2.refresh_failed` log has the same code. |
| `grant_context.cache` | Whether the permit came from ExtProc's permit cache (`hit`) or from a broker evaluation (`miss`). A request that waits for the evaluation of another request is a `miss`. |
| `http.request.method` | HTTP method: an allowlisted method, `_OTHER` otherwise. |
| `http.response.status_code` | HTTP status code of the response. On `extproc.client_assertion.refresh`, it is set only from an error response of the token endpoint, and is `0` otherwise. On `tokenexchange.access_evaluation`, it is set only on an `error` decision. |
| `impersonation.target_agent_id` | ID of the agent that the impersonation request names as its audience. |
| `oauth2.refresh.outcome` | Result of the background refresh: `refreshed` when it stored new tokens, `not_due` when no refresh was needed or the session no longer exists, `failed` otherwise. |
| `oauth2.refresh.triggered_by` | Always `background`, because a token exchange started the refresh. |
| `outcome` | Result of the operation that the metric records. Each metric has its own values. |
| `refreshable` | Whether the stored session can be refreshed. |
| `service_id` | Canonical ID of the service when one is set, otherwise its UUID. |
| `storage.outcome` | Result of a storage call that failed in a way the caller handles: `not_found`, `conflict`, `validation` or `canceled`. Absent on success. The span status stays unset. |
| `token_exchange.agent.id` | ID of the agent that the exchange or the access evaluation is for. Set on third-party exchanges and on access evaluations, after the broker resolves a registered agent. |
| `token_exchange.broker_status_code` | HTTP status code of the broker's response. Set when the broker answered with a status other than 200. |
| `token_exchange.dependency` | Dependency that the operation used: ExtProc itself (`local`), the broker, or the OAuth provider. |
| `token_exchange.exchange_kind` | Kind of exchange: a third-party service token or user impersonation. ExtProc always sets `unknown`. |
| `token_exchange.failure_detail` | Detail code of the failure. An unclassified failure has `internal_unclassified`. A successful metric observation and a successful `extproc.grant_context.*` span have `none`. |
| `token_exchange.failure_stage` | Step of the exchange that failed. A successful metric observation has `none`. |
| `token_exchange.grant.id` | ID of the user grant for the agent. Set on third-party exchanges and on access evaluations when the broker found the grant, also when it is expired. |
| `token_exchange.grant.updated_at` | Time of the last change to the grant, in UTC as RFC 3339. Set on third-party exchanges and on access evaluations. |
| `token_exchange.grant.valid_until` | Expiry time of the grant, in UTC as RFC 3339. Set on third-party exchanges, and absent when the grant does not expire. |
| `token_exchange.oauth_error_code` | OAuth error code of the response that rejected the operation, such as `invalid_grant`. A missing code, or one that is not on the allowlist, is `unknown`. `extproc.client_assertion.refresh` also sets it on success, as `unknown`. |
| `token_exchange.operation` | ExtProc operation that failed. `extproc.client_assertion.refresh` also sets it on success, as `assertion_refresh`. The `extproc.grant_context.*` spans always set `grant_resolution`, and `tokenexchange.access_evaluation` always sets `access_evaluation`. |
| `token_exchange.outcome` | Result class of the exchange, the access evaluation or the grant context resolution, such as `success`, `authorization_denied` or `reauth_required`. |
| `token_exchange.recovery_action` | Recommended way to repair the failure. It is a diagnostic hint, and no component acts on it. |
| `token_exchange.recovery_target` | What the recovery action applies to, such as the consent or the provider session. |
| `token_exchange.service.id` | UUID of the registered service. Unlike the metric attribute `service_id`, it is never the canonical ID. |
| `token_exchange.session.dependency` | Dependency of the failed session operation, such as the provider or the session repository. |
| `token_exchange.session.detail` | Detail code of the failed session operation. |
| `token_exchange.session.kind` | Class of the session failure, such as a provider or an infrastructure failure. |
| `token_exchange.session.oauth_code` | OAuth error code that the provider returned. A code that is not on the allowlist is `unknown`, and the key is absent when the provider returned no code. |
| `token_exchange.session.operation` | Session operation that failed. |
| `token_exchange.session.status_code` | HTTP status code that the provider returned. Absent when the failure has none. |
| `triggered_by` | Source of the refresh: `background` when a token exchange found the access token close to expiry, `sweep` when the admin session sweep refreshed the session. |

## Attribute values

### internal/domain/oauth2session.Dependency

`credential_source`, `encryption`, `none`, `provider`, `provider_repository`, `session_repository`, `signing`

### internal/domain/oauth2session.ErrorDetail

`access_token_expired`, `caller_canceled`, `configuration`, `credential_source_unavailable`, `decryption_failed`, `encryption_failed`, `internal_unclassified`, `persistence_failed`, `provider_client_rejected`, `provider_rejected`, `provider_response_invalid`, `provider_unavailable`, `refresh_rejected`, `refresh_unavailable`, `repository_unavailable`, `session_missing`

### internal/domain/oauth2session.ErrorKind

`canceled`, `configuration`, `infrastructure`, `internal`, `provider`, `session`

### internal/domain/oauth2session.Operation

`code_exchange`, `refresh`, `scope_validation`, `session_lookup`, `unknown`

### internal/domain/oauth2session.SessionHealthExpiry

| Value | Meaning |
|---|---|
| `expired` | The access token has already expired. |
| `expiring` | The access token expires within the refresh lookahead. |
| `unknown` | No access-token expiry is recorded. |
| `valid` | The access token expires after now plus the refresh lookahead. |

### internal/domain/storage.ApprovalPersistence

`once`, `permanent`, `session`

### internal/domain/storage.ApprovalStatus

`approved`, `denied`, `pending`

### internal/domain/storage.ErrorKind

| Value | Meaning |
|---|---|
| `conflict` | Operation conflicts with existing data. |
| `connection` | Storage backend connection failed or disconnected. |
| `not_found` | Requested entity does not exist. |
| `timeout` | Operation exceeded its timeout deadline. |
| `unknown` | Error cause is unknown or unexpected. |
| `validation` | Input validation failed. |

### internal/domain/tokenexchange.ExchangeKind

`impersonation`, `third_party`

### internal/domain/tokenexchange.FailureDetail

`access_token_expired`, `agent_invalid`, `agent_missing`, `agent_repository_unavailable`, `caller_canceled`, `cel_configuration`, `cel_evaluation_failed`, `client_expired`, `client_invalid`, `client_policy_denied`, `credential_identity_changed`, `credential_source_unavailable`, `grant_empty`, `grant_expired`, `grant_missing`, `grant_permission_set_missing`, `grant_permission_set_undeclared`, `grant_repository_unavailable`, `grant_scope_intersection_empty`, `grant_service_definition_missing`, `grant_service_omitted`, `grant_service_requirement_excluded`, `internal_unclassified`, `jwks_unavailable`, `jwt_configuration`, `none`, `provider_client_rejected`, `provider_rejected`, `provider_response_invalid`, `provider_unavailable`, `refresh_rejected`, `refresh_unavailable`, `request_malformed`, `resource_ambiguous`, `resource_missing`, `resource_repository_unavailable`, `resource_unregistered`, `response_write_failed`, `session_configuration`, `session_decryption_failed`, `session_encryption_failed`, `session_missing`, `session_persistence_failed`, `session_repository_unavailable`, `session_scope_insufficient`, `subject_category_ineligible`, `subject_category_unsupported`, `subject_expired`, `subject_invalid`

### internal/domain/tokenexchange.FailureStage

`client_authorization`, `client_validation`, `exchange_routing`, `grant_authorization`, `identity_resolution`, `none`, `refresh`, `request_validation`, `resource_resolution`, `response_write`, `scope_validation`, `session_lookup`, `subject_validation`

### internal/domain/tokenexchange.Outcome

`authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success`

### internal/domain/tokenexchange.RecoveryAction

`fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry`

### internal/domain/tokenexchange.RecoveryTarget

`broker_configuration`, `calling_client`, `consent`, `none`, `provider_session`, `subject_identity`

### internal/extproc/server.Dependency

`broker`, `local`, `oauth_provider`

### internal/extproc/server.ErrorKind

`assertion_expired`, `caller_canceled`, `circuit_open`, `configuration`, `internal`, `invalid_response`, `none`, `rejected`, `unavailable`

### internal/extproc/server.ExchangeKind

`impersonation`, `third_party`, `unknown`

### internal/extproc/server.FailureDetail

`authorization_denied`, `broker_response_invalid`, `broker_unavailable`, `caller_canceled`, `circuit_open`, `client_expired`, `client_invalid`, `configuration`, `consent_required`, `credential_rejected`, `grant_context_denied`, `grant_context_unavailable`, `internal_unclassified`, `none`, `policy_configuration`, `policy_denied`, `policy_evaluation_failed`, `provider_response_invalid`, `provider_unavailable`, `reauth_required`, `request_malformed`, `resource_missing`, `response_write_failed`, `session_reauth_required`, `subject_invalid`

### internal/extproc/server.FailureStage

`client_authorization`, `exchange_routing`, `none`, `refresh`, `request_validation`, `response_write`

### internal/extproc/server.Operation

`assertion_refresh`, `configuration`, `exchange`

### internal/extproc/server.Outcome

`authentication_failed`, `authorization_denied`, `canceled`, `configuration_error`, `infrastructure_error`, `invalid_request`, `reauth_required`, `success`

### internal/extproc/server.RecoveryAction

`fix_configuration`, `none`, `reauthenticate`, `reconsent`, `retry`

### internal/extproc/server.RecoveryTarget

`consent`, `none`, `provider_session`
