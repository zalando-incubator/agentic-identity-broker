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
| [`extproc.grant_context.resolve`](#extprocgrant_contextresolve) | Internal | `extproc` | Yes | Returns the delegation permit for the subject token and the requested resource, from ExtProc's permit cache or from the broker. With `grant_context.enabled`, ExtProc resolves it once per request, in the headers phase, and uses it for every OPA decision of the request, including each message of an MCP batch. A permit that expires before a decision fails the request; ExtProc does not resolve it again. |
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
| [`tokenexchange.impersonation.opa_evaluate`](#tokenexchangeimpersonationopa_evaluate) | Internal | `tokenexchange` | Yes | Evaluates one impersonation rule's OPA decision on the policy revision the request read before its first rule. An explicit deny lets later rules run; an invalid decision, an error, a timeout or a cancellation stops the request. |
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
| [`tokenexchange.impersonation_opa.bundle_download_age`](#tokenexchangeimpersonation_opabundle_download_age) | Float64ObservableGauge | `s` | `tokenexchange` | Seconds since each impersonation policy bundle was last downloaded successfully |
| [`tokenexchange.impersonation_opa.bundle_downloads`](#tokenexchangeimpersonation_opabundle_downloads) | Int64Counter | `{download}` | `tokenexchange` | Impersonation policy bundle download attempts by bundle and outcome |
| [`tokenexchange.impersonation_opa.revision_age`](#tokenexchangeimpersonation_oparevision_age) | Float64ObservableGauge | `s` | `tokenexchange` | Seconds since the active impersonation policy revision was admitted |
| [`tokenexchange.impersonation_opa.revisions`](#tokenexchangeimpersonation_oparevisions) | Int64Counter | `{revision}` | `tokenexchange` | Candidate impersonation policy revisions by outcome and reason |

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

Returns the delegation permit for the subject token and the requested resource, from ExtProc's permit cache or from the broker. With `grant_context.enabled`, ExtProc resolves it once per request, in the headers phase, and uses it for every OPA decision of the request, including each message of an MCP batch. A permit that expires before a decision fails the request; ExtProc does not resolve it again.

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

### tokenexchange.impersonation.opa_evaluate

Evaluates one impersonation rule's OPA decision on the policy revision the request read before its first rule. An explicit deny lets later rules run; an invalid decision, an error, a timeout or a cancellation stops the request.

Started in `internal/adapters/policy/opa/revision.go`.

| Attribute | Values |
|---|---|
| `error.type` | [`internal/adapters/policy/opa.evaluationErrorType`](#internaladapterspolicyopaevaluationerrortype) |
| `impersonation.authorization.action` | [`internal/adapters/policy/opa.evaluationAction`](#internaladapterspolicyopaevaluationaction) |
| `impersonation.rule` | any `string` |

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

### tokenexchange.impersonation_opa.bundle_download_age

Created in `internal/adapters/policy/opa/telemetry.go`.

| Attribute | Values |
|---|---|
| `bundle` | any `string` |

### tokenexchange.impersonation_opa.bundle_downloads

Created in `internal/adapters/policy/opa/telemetry.go`.

| Attribute | Values |
|---|---|
| `bundle` | any `string` |
| `outcome` | [`internal/adapters/policy/opa.downloadOutcome`](#internaladapterspolicyopadownloadoutcome) |

### tokenexchange.impersonation_opa.revision_age

Created in `internal/adapters/policy/opa/telemetry.go`.

No attributes.

### tokenexchange.impersonation_opa.revisions

Created in `internal/adapters/policy/opa/telemetry.go`.

| Attribute | Values |
|---|---|
| `outcome` | `admitted`, `rejected` |
| `reason` | `none`, [`internal/adapters/policy/opa.revisionReason`](#internaladapterspolicyoparevisionreason) |

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
| `bundle` | Name of an impersonation policy bundle, from the bundles of the configured OPA bootstrap. |
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
| `impersonation.authorization.action` | Outcome of the OPA decision. |
| `impersonation.rule` | Name of the impersonation rule whose OPA decision the span evaluates, from the configured rule names. |
| `impersonation.target_agent_id` | ID of the agent that the impersonation request names as its audience. |
| `oauth2.refresh.outcome` | Result of the background refresh: `refreshed` when it stored new tokens, `not_due` when no refresh was needed or the session no longer exists, `failed` otherwise. |
| `oauth2.refresh.triggered_by` | Always `background`, because a token exchange started the refresh. |
| `outcome` | Result of the operation that the metric records. Each metric has its own values. |
| `reason` | Why a candidate impersonation policy revision was rejected, or `none` when it was admitted. |
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

### internal/adapters/policy/opa.downloadOutcome

| Value | Meaning |
|---|---|
| `failure` | Set when the download failed; the active revision stays and ages. |
| `success` | Set when the bundle was downloaded or was unchanged. |

### internal/adapters/policy/opa.evaluationAction

| Value | Meaning |
|---|---|
| `allow` | Set when the decision allowed the request. |
| `canceled` | Set when the caller canceled the request during evaluation. |
| `deny` | Set when the decision explicitly denied the request; later rules still run. |
| `error` | Set when evaluation failed or timed out; the request failed with server_error. |
| `invalid` | Set when the decision was undefined, malformed, or an unsupported action; the request failed with server_error. |

### internal/adapters/policy/opa.evaluationErrorType

| Value | Meaning |
|---|---|
| `opa_decision_invalid` | Set when the decision was not exactly one allow or deny object. |
| `opa_evaluation_failed` | Set when evaluation raised an error or timed out. |

### internal/adapters/policy/opa.revisionReason

| Value | Meaning |
|---|---|
| `bundles_incomplete` | Set when the candidate does not contain every configured bundle. |
| `compile_failed` | Set when a module does not parse or compile; fix the policy and publish a new revision. |
| `decision_absent` | Set when an OPA rule's decision rule does not exist in the candidate; the log names the first failing rule. |
| `none` | Set on an admitted revision. |
| `prohibited_builtin` | Set when a module calls a removed nondeterministic built-in such as http.send, even in a rule no decision reaches. |
| `snapshot_failed` | Set when the candidate's policy or data cannot be read or prepared. |

### internal/domain/oauth2session.Dependency

| Value | Meaning |
|---|---|
| `credential_source` | The service's credential files are not configured or could not be read, or the recorded client ID is missing or differs. |
| `encryption` | Encrypting or decrypting the session's tokens or the service's client secret failed. |
| `none` | The failure is not attributed to a dependency, as for most configuration errors, cancellations and unclassified failures. |
| `provider` | The provider's token endpoint failed, rejected the request or sent an invalid response. |
| `provider_repository` | The broker could not load the service registration, because storage failed or the service is missing. |
| `session_repository` | The failure concerns the stored session: storage failed, or the session is missing or has no refresh token. |
| `signing` | The broker had no usable key to sign the client assertion, or signing it failed. |

### internal/domain/oauth2session.ErrorDetail

| Value | Meaning |
|---|---|
| `access_token_expired` | The access token has expired and the session has no refresh token, so the user must reconnect to the service. |
| `caller_canceled` | The request was canceled, or its deadline passed, before the session operation finished. A refresh that other requests share keeps running and reports its own result. |
| `configuration` | The service's OAuth2 registration is missing, invalid or incomplete, so fix the service configuration. |
| `credential_source_unavailable` | No credential files are configured for the service, the broker could not read them, or the recorded client ID is missing or differs from the configured one. A token exchange reports the client ID cases as `credential_identity_changed`. |
| `decryption_failed` | The broker could not decrypt the session's stored tokens or the service's client secret, so check the encryption keys. |
| `encryption_failed` | The broker could not encrypt new tokens from the provider before storing them, so check the encryption keys. Starting an authorization also logs it when the broker cannot encrypt the authorization state. |
| `internal_unclassified` | The broker could not classify the failure into any other detail. |
| `persistence_failed` | The broker could not save new tokens, or a recorded refresh rejection, to the session, so check the storage backend. The session sweep stops at this failure instead of counting it. |
| `provider_client_rejected` | The provider answered with a 4xx `invalid_client` or `unauthorized_client` error, so fix the client credentials or the client registration of the service. |
| `provider_rejected` | The provider's token endpoint refused the request with a 4xx error that is neither a client rejection nor a refresh `invalid_grant`. At the callback, this includes an authorization code that the provider no longer accepts. |
| `provider_response_invalid` | The provider answered a refresh with a success status but a malformed or oversized body, or one without an access token. |
| `provider_unavailable` | The provider's token endpoint was unreachable, timed out or answered 429 or 5xx, or signing the client assertion failed. The dependency is `signing` for the signing case. |
| `refresh_rejected` | The provider answered a refresh with a 4xx `invalid_grant` error, now or earlier, so the user must reconnect to the service. The broker records the rejection and stops refreshing the session until the user reconnects. |
| `refresh_unavailable` | A refresh was due or requested but the session has no refresh token, so the user must reconnect to the service; if the access token has already expired when a refresh is due, the detail is `access_token_expired` instead. |
| `repository_unavailable` | The broker could not read or lock the stored session, or read the service registration, so check the storage backend. The session sweep stops at this failure instead of counting it. |
| `session_missing` | No session is stored for the user and the service, so the user must connect to the service. The background refresh and the session sweep skip such sessions instead of counting a failure. |

### internal/domain/oauth2session.ErrorKind

| Value | Meaning |
|---|---|
| `canceled` | The request was canceled, or its deadline passed, before the session operation finished. |
| `configuration` | The service's configuration is wrong, including a provider that rejects the broker's client, so fix the configuration. |
| `infrastructure` | A dependency failed: the provider was unavailable, or storage, encryption, signing or the credential files failed. |
| `internal` | The broker could not classify the failure. |
| `provider` | The provider rejected the request, including a rejected refresh token, or answered a refresh with an invalid response. |
| `session` | No session is stored, or a due refresh is impossible because the session has no refresh token, so the user must connect to the service. |

### internal/domain/oauth2session.Operation

| Value | Meaning |
|---|---|
| `code_exchange` | Connecting the user to the service failed, while starting the authorization or while exchanging the authorization code on the callback. Token exchanges never report it. |
| `refresh` | The broker could not renew the access token with the stored refresh token, or could not save the renewed tokens. |
| `scope_validation` | Reserved and currently unused; a token exchange reports missing session scopes as `session_scope_insufficient` in `token_exchange.failure_detail`. |
| `session_lookup` | The broker failed to read or decrypt the stored session of the user for the service, or found none. |
| `unknown` | The broker could not tell which session operation failed; it comes with the `internal_unclassified` detail. |

### internal/domain/oauth2session.SessionHealthExpiry

| Value | Meaning |
|---|---|
| `expired` | The access token has already expired. |
| `expiring` | The access token expires within the refresh lookahead. |
| `unknown` | No access-token expiry is recorded. |
| `valid` | The access token expires after now plus the refresh lookahead. |

### internal/domain/storage.ApprovalPersistence

| Value | Meaning |
|---|---|
| `once` | The approval covers one call to the tool with the approved arguments, and ExtProc consumes it when it lets that call through. |
| `permanent` | The decision covers matching tool calls until the user revokes it, and a permanent denial blocks the tool for any arguments. |
| `session` | The approval covers matching tool calls in the agent session it was requested in, while that session is active. |

### internal/domain/storage.ApprovalStatus

| Value | Meaning |
|---|---|
| `approved` | The user approved the tool call, and the approval lets matching calls through within its persistence scope. |
| `denied` | The user denied the tool call or revoked a permanent decision, and only a permanent denial keeps blocking the tool. |
| `pending` | The approval waits for the user's decision, and it expires if the user does not decide within `approvals.pending_ttl`. ExtProc answers the tool call with the approval URL at once, so the agent calls the tool again after the decision. |

### internal/domain/storage.ErrorKind

| Value | Meaning |
|---|---|
| `conflict` | The write conflicts with stored data: a unique value is already taken, the transaction failed to serialize, the stored version is newer, or a referenced row is missing or was deleted concurrently. Storage spans record it as `storage.outcome`, because the caller handles it. |
| `connection` | The database was not initialized, or a query, the start of a transaction or a commit failed in a way that the repository does not classify further, so check the database and its connections. Storage spans record it as `error.type`. |
| `not_found` | The requested row does not exist, an update or a delete matched no row, or a write referenced a row that does not exist. Storage spans record it as `storage.outcome`, because the caller handles it. |
| `timeout` | The operation passed its deadline, from `storage.timeouts.read`, `storage.timeouts.write` or the caller, or PostgreSQL canceled a statement after its statement timeout. Storage spans record it as `error.type`. |
| `unknown` | The repository could not classify the failure, such as a stored value that it could not decode, a row count that it could not read or a broken invariant. Storage spans also record it as `error.type` for an error that is not a storage error. |
| `validation` | The repository rejected the input, such as an empty ID, a nil entity, a value that it could not encode as JSON or a value that a check constraint refuses. Storage spans record it as `storage.outcome`, because the caller handles it. |

### internal/domain/tokenexchange.ExchangeKind

| Value | Meaning |
|---|---|
| `impersonation` | The `audience` parameter selected a target agent, so the broker mints its own token for the subject instead of returning a provider token. |
| `third_party` | The caller exchanged a subject token for the user's stored access token to a registered service. The broker also uses it when token exchange is not enabled. |

### internal/domain/tokenexchange.FailureDetail

| Value | Meaning |
|---|---|
| `access_token_expired` | The stored access token has expired and the session has no refresh token, so the user must reconnect the service. |
| `agent_invalid` | The agent ID from the token claims is not a valid agent UUID, or the client ID that it is resolved from matches several registered agents. |
| `agent_missing` | No registered agent matches the agent ID that the token claims resolve to, or that the impersonation `audience` names, in which case the caller may have named the wrong agent. |
| `agent_repository_unavailable` | Looking up the agent in storage failed for a reason other than the agent not existing. |
| `caller_canceled` | The request was canceled or its deadline passed before the broker finished it, usually because the caller disconnected. |
| `cel_configuration` | A configured CEL expression returned a value of the wrong type or an empty value, so check the expression and the token claims that it reads; impersonation reports such values as `cel_evaluation_failed` instead. A CEL expression that fails to compile stops the broker at startup. |
| `cel_evaluation_failed` | A CEL expression failed or timed out while it ran, such as when it reads a claim that the token lacks, or an impersonation expression returned a value that is not a string or an empty identity. |
| `client_expired` | The client assertion has expired, so the calling client must create a new one. |
| `client_invalid` | The client assertion failed validation, is not trusted or lacks a required subject, or an access evaluation request carried no bearer assertion. |
| `client_policy_denied` | The CEL authorization policy, or an impersonation rule's authorization predicate, denied the calling client. |
| `credential_identity_changed` | The service's configured client ID differs from the one that the session recorded, or the session recorded none, so the user must reconnect. |
| `credential_source_unavailable` | No credential files are configured for the service, or the broker could not read them during refresh. |
| `grant_empty` | The user's grant for the agent has no permission set entries. |
| `grant_expired` | The user's grant for the agent has passed its expiry time, so the user must renew it on the agent's consent page. |
| `grant_missing` | The user has not granted the agent access or has revoked the grant, so the user must consent on the agent's consent page. |
| `grant_permission_set_missing` | The grant references a permission set that no longer exists, even one unrelated to the requested service. |
| `grant_permission_set_undeclared` | The grant references a permission set that the agent no longer declares. |
| `grant_repository_unavailable` | The broker could not read the user's grant, or the permission sets that it references, from storage. |
| `grant_scope_intersection_empty` | None of the scopes granted for the service is in the agent's required scopes for it, so check those required scopes. |
| `grant_service_definition_missing` | The grant includes the requested service, but none of the permission sets that include it defines that service. |
| `grant_service_omitted` | None of the grant's permission set entries includes the requested service. |
| `grant_service_requirement_excluded` | The grant covers the requested service, but the agent declares service requirements that do not list it, so check those requirements. |
| `internal_unclassified` | The broker has no specific classification for the failure, and the failure stage shows where it happened. |
| `jwks_unavailable` | The broker could not fetch the signing keys that it needs to verify the subject token, an actor token or the client assertion. |
| `jwt_configuration` | The token validation settings, such as issuers, audience or clock skew, are invalid, which stops the broker at startup instead of failing a request. |
| `none` | The exchange or access evaluation succeeded, and broker spans and logs then leave out the failure detail. |
| `opa_decision_invalid` | An impersonation rule's OPA decision was undefined for the request or was not exactly one allow or deny object. Give the policy a default decision. Later rules are not tried. |
| `opa_evaluation_failed` | An impersonation rule's OPA policy failed, raised a built-in error, or timed out while it ran. The request fails without trying later rules. |
| `provider_client_rejected` | The provider answered the refresh with `invalid_client` or `unauthorized_client`, so fix the client credentials or registration of the service. |
| `provider_rejected` | The provider refused the refresh with a 4xx error that is neither a client rejection nor `invalid_grant`. |
| `provider_response_invalid` | The provider answered the refresh with a success status but a malformed or oversized body, or one without an access token. |
| `provider_unavailable` | The provider's token endpoint was unreachable, timed out or answered 429 or 5xx during refresh, or signing the client assertion failed. |
| `refresh_rejected` | The provider rejected the session's refresh token with `invalid_grant`, now or on an earlier refresh, so the user must reconnect the service. |
| `refresh_unavailable` | The broker had to renew the access token but found no usable refresh token, so the user must reconnect the service. |
| `request_malformed` | A parameter or the request body was missing, repeated or invalid, repeated `resource` values differ, an impersonation asked for a scope that the target agent does not allow, or the `context.agent` of an access evaluation does not match the subject token. |
| `resource_ambiguous` | More than one registered service claims the requested resource; storage keeps each resource unique to one service, so the broker does not currently set it. |
| `resource_missing` | The third-party exchange request had no `resource` parameter. |
| `resource_repository_unavailable` | Looking up the service for the resource failed in storage, or an access evaluation could not load canonical service IDs. |
| `resource_unregistered` | No registered service lists the requested resource among its protected resources. |
| `response_write_failed` | The broker could not encode or fully write its response to the client. |
| `session_configuration` | The service's OAuth2 registration is missing, invalid or incomplete for a refresh, or no key is configured to sign client assertions. |
| `session_decryption_failed` | The broker could not decrypt the session's stored tokens or the service's client secret, so check the encryption keys. |
| `session_encryption_failed` | The broker could not encrypt the tokens from a refresh before storing them, so check the encryption keys. |
| `session_missing` | The user has no stored session for the service, so the user must connect it through the `error_uri` of the `invalid_grant` response. |
| `session_persistence_failed` | The broker could not save refreshed tokens, or a recorded refresh rejection, to the session store. |
| `session_repository_unavailable` | The broker could not read or lock the stored session, or read the service registration for a refresh. |
| `session_scope_insufficient` | The user's stored session lacks scopes that the grant gives the agent for the service, so the user must reconnect the service, which helps only if the service registration requests those scopes. |
| `subject_category_ineligible` | An impersonation rule does not allow the subject's category, human or agent, or the subject is a different registered agent. |
| `subject_category_unsupported` | The agent-subject expression matched, so the subject token is an agent's own token, which third-party exchange and access evaluation reject. |
| `subject_expired` | The subject token, or an impersonation actor token, has expired, so the caller needs a new one. |
| `subject_invalid` | The subject token, or an impersonation actor token, failed parsing or its signature, issuer, audience or claim checks. |

### internal/domain/tokenexchange.FailureStage

| Value | Meaning |
|---|---|
| `client_authorization` | The broker was evaluating the authorization policy for the calling client, or the subject categories that an impersonation rule allows. |
| `client_validation` | The broker was validating the client assertion that authenticates the calling client, including fetching its signing keys. |
| `exchange_routing` | The failure is not tied to one exchange step, such as when token exchange is not enabled or an impersonation token cannot be minted. The broker also uses it for an error that carries no stage. |
| `grant_authorization` | The broker was checking that the user's grant for the agent exists, has not expired and covers the requested service. |
| `identity_resolution` | The broker was deriving identities and the subject category from token claims with CEL, or looking up the agent. |
| `none` | The exchange or access evaluation succeeded, and broker spans and logs then leave out the failure stage. |
| `refresh` | The broker was renewing the session's access token with its refresh token, or found that it cannot. |
| `request_validation` | The broker was checking the request's parameters or body, such as the grant type, token types, resource or audience. |
| `resource_resolution` | The broker was looking up the registered service whose protected resources include the requested resource. An access evaluation also uses it while loading canonical service IDs. |
| `response_write` | The broker was encoding or writing its response to the client. |
| `scope_validation` | The broker was checking that the session's scopes cover the scopes that the grant gives the agent for the service. |
| `session_lookup` | The broker was reading and decrypting the user's stored session for the service. |
| `subject_validation` | The broker was validating the subject token, or an impersonation actor token, including fetching its signing keys. |

### internal/domain/tokenexchange.Outcome

| Value | Meaning |
|---|---|
| `authentication_failed` | The subject token, an impersonation actor token or the client assertion was invalid or expired. |
| `authorization_denied` | The credentials were valid, but the authorization policy, the subject category or the user's grant did not allow the exchange. |
| `canceled` | The request was canceled or its deadline passed before the broker finished it, usually because the caller disconnected. |
| `configuration_error` | The broker configuration or an agent or service registration prevents the exchange, so a retry fails the same way until an operator fixes it. |
| `infrastructure_error` | Storage, the signing key set, encryption, the provider, a CEL evaluation or the response write failed, or the failure is unclassified. |
| `invalid_request` | The request was malformed, had no resource, or named a resource that no service registers. |
| `reauth_required` | The user's stored session with the service is missing or unusable, so the broker answered `invalid_grant` with an `error_uri` to reconnect it. |
| `success` | The broker issued the requested token, or an access evaluation permitted the request. |

### internal/domain/tokenexchange.RecoveryAction

| Value | Meaning |
|---|---|
| `fix_configuration` | An operator must correct the broker configuration or an agent or service registration before the exchange can succeed. |
| `none` | The broker suggests no repair, as for a success, an invalid request, a policy or subject category denial, or a cancellation. |
| `reauthenticate` | A fresh credential is needed, which the recovery target names: a subject token, a client assertion or the user's session with the service. |
| `reconsent` | A grant check denied the request, so the user should review the grant on the consent page that the response links to, unless the failure detail points to the agent's service requirements. |
| `retry` | The failure is an infrastructure failure or unclassified, so a later request may succeed if its cause was temporary. |

### internal/domain/tokenexchange.RecoveryTarget

| Value | Meaning |
|---|---|
| `broker_configuration` | The broker configuration or an agent or service registration, which an operator must fix. |
| `calling_client` | The calling client's client assertion, which the client must replace with a new, valid one. |
| `consent` | The user's grant for the agent, which the user manages on the agent's consent page; it goes with `reconsent`. |
| `none` | The recovery action has no target, which holds for `none` and `retry`. |
| `provider_session` | The user's stored session with the service, which the user reconnects through the `error_uri` of the `invalid_grant` response. |
| `subject_identity` | The subject token, or an impersonation actor token, which the caller must replace with a new, valid one. |

### internal/extproc/server.Dependency

| Value | Meaning |
|---|---|
| `broker` | The broker's token endpoint, which ExtProc calls for the token exchange behind its circuit breaker. |
| `local` | ExtProc itself, without an outbound call, such as its configuration or its expired client assertion; ExtProc also uses it for an unknown dependency. |
| `oauth_provider` | The OAuth provider that issues ExtProc's client assertion through the client credentials grant, not a provider behind the broker. |

### internal/extproc/server.ErrorKind

| Value | Meaning |
|---|---|
| `assertion_expired` | ExtProc's own client assertion had expired when an exchange needed it, so ExtProc answered 503 without calling the broker. |
| `caller_canceled` | The request was canceled, or its deadline passed, while ExtProc waited for the exchange; ExtProc sets no error status for it. |
| `circuit_open` | ExtProc's circuit breaker for the broker was open, or half-open with no free probe, so ExtProc answered 503 without calling the broker; failures inside ExtProc also count toward opening it. |
| `configuration` | ExtProc's own configuration is unusable, such as a client assertion type that it does not support or a token endpoint that it cannot build a request for. |
| `internal` | ExtProc could not classify the error; it is also the fallback for an unknown kind. |
| `invalid_response` | The broker or the OAuth provider answered with a body that ExtProc could not use, or the delegation permit carried an invalid grant timestamp. |
| `none` | Nothing failed; ExtProc never sets `error.type` to it. |
| `rejected` | The broker or the OAuth provider answered with a 4xx error, or ExtProc rejected the request's token-exchange metadata and answered 503. |
| `unavailable` | The broker or the OAuth provider was unreachable, timed out or answered 5xx or 429, or ExtProc had no usable grant context. |

### internal/extproc/server.ExchangeKind

| Value | Meaning |
|---|---|
| `impersonation` | A user impersonation exchange; ExtProc never sets it, because it cannot observe the broker's exchange kind. |
| `third_party` | A third-party service token exchange; ExtProc never sets it, because it cannot observe the broker's exchange kind. |
| `unknown` | ExtProc cannot observe which kind of exchange the broker performed; ExtProc always sets this value. |

### internal/extproc/server.FailureDetail

| Value | Meaning |
|---|---|
| `authorization_denied` | The broker, or the OAuth provider during refresh, answered `access_denied` without an `error_uri`, so ExtProc had no consent URL to return. |
| `broker_response_invalid` | The broker answered 200, but ExtProc could not read or parse the body, or the body had no access token. |
| `broker_unavailable` | ExtProc could not reach the broker in time, or the broker answered with a 5xx or 429 status or with `server_error` or `temporarily_unavailable`. |
| `caller_canceled` | The request was canceled, or its deadline passed, while ExtProc waited for the exchange or was sending its response. |
| `circuit_open` | ExtProc's circuit breaker for the broker was open, or half-open with no free probe, so ExtProc answered 503 without calling the broker; failures inside ExtProc, such as an expired client assertion, also count toward opening it. |
| `client_expired` | ExtProc's own client assertion had expired, so ExtProc answered 503 without calling the broker; look for failed `extproc.client_assertion.refresh` spans. |
| `client_invalid` | The broker or the OAuth provider answered `invalid_client` or `unauthorized_client`: it rejected ExtProc's client authentication, or ExtProc may not use that grant. |
| `configuration` | ExtProc's own configuration is unusable: it could not build the request to the broker, or the client assertion type is neither `id_token` nor `access_token`. |
| `consent_required` | The broker answered `access_denied` with an `error_uri`, and ExtProc returned that URL to the client in a 403 `insufficient_scope` challenge. |
| `credential_rejected` | The broker answered `invalid_grant` without an `error_uri`, which it does only when the subject token fails validation, or the OAuth provider did so during refresh. The client gets no sign-in URL. |
| `grant_context_denied` | The broker's grant context evaluation denied the delegation, and ExtProc answered 403 `access_denied` with the consent URL as `error_uri`. |
| `grant_context_unavailable` | ExtProc had no usable delegation permit and answered 503: the broker evaluation failed or could not run, the request was canceled meanwhile, the permit expired or was invalid, or the exchange result did not match it. |
| `internal_unclassified` | ExtProc could not classify the failure, such as an unexpected error or an OAuth error code that it does not map to a detail. |
| `none` | Nothing failed; metrics of successful requests and successful `extproc.grant_context.*` spans carry it, while other successful spans and logs leave out the failure detail. |
| `policy_configuration` | Authorization is enabled but the gateway sent no protocol metadata, or no `mcp_server` metadata for an MCP request; ExtProc answered 403. |
| `policy_denied` | The OPA decision did not allow the request, also after a policy error, a timeout or an undefined result, so ExtProc answered 403 `access_denied`; `authorization.result_code` on `extproc.opa.evaluate` tells which, and for a request with a body only logs carry this detail. |
| `policy_evaluation_failed` | The authorizer returned an error instead of a decision; the OPA authorizer never does, because it reports errors, timeouts and undefined results as `policy_denied`. |
| `provider_response_invalid` | The OAuth provider answered the client assertion refresh with a response that ExtProc could not read or parse, or without the configured `id_token` or `access_token`. |
| `provider_unavailable` | ExtProc could not reach the OAuth provider in time to refresh its client assertion, or the provider answered 5xx, 429, `server_error` or `temporarily_unavailable`. |
| `reauth_required` | An OAuth error had the code `reauth_required` and no `error_uri`; the broker never sends this code, so it can only come from the OAuth provider during refresh. |
| `request_malformed` | ExtProc found the request's metadata, method or body malformed, or the broker or the OAuth provider answered `invalid_request`, `invalid_target`, `invalid_scope` or `unsupported_grant_type`. |
| `resource_missing` | The request's token-exchange metadata has no resource URI, or a blank one; check that the gateway sets it, because ExtProc answers 503. |
| `response_write_failed` | ExtProc could not send its response to the gateway on the processing stream; only logs carry it, since the span has already ended. |
| `session_reauth_required` | The broker answered `invalid_grant` with an `error_uri` because the user must reconnect the provider session, and ExtProc returned that URL to the client in a 403 `insufficient_scope` challenge. |
| `subject_invalid` | The broker, or the OAuth provider during refresh, answered `invalid_token`; the broker reports a rejected subject token as `invalid_grant`, which is `credential_rejected`. |

### internal/extproc/server.FailureStage

| Value | Meaning |
|---|---|
| `client_authorization` | ExtProc's own authorization step failed or denied the request: the OPA policy, the gateway metadata that it needs, or the grant context. |
| `exchange_routing` | The token exchange with the broker failed, including an open circuit breaker and a canceled wait; ExtProc also uses it for an unknown stage. |
| `none` | Nothing failed; metrics of successful requests carry it, while successful spans and logs leave out the failure stage. |
| `refresh` | ExtProc's own client assertion refresh failed, or an exchange found the assertion expired; it is not the broker's refresh of a provider session. |
| `request_validation` | ExtProc rejected the request itself: its token-exchange metadata, its HTTP method or its body was missing or malformed. |
| `response_write` | ExtProc could not send its response to the gateway; only logs carry it, since the span has already ended. |

### internal/extproc/server.Operation

| Value | Meaning |
|---|---|
| `assertion_refresh` | The refresh of ExtProc's own client assertion at the OAuth provider; an exchange that finds the assertion expired also reports it. |
| `configuration` | Building the token exchanger at startup; a failure stops ExtProc, so no span or metric carries this value. |
| `exchange` | The token exchange with the broker, including the wait for a shared exchange; ExtProc also uses it for an unknown operation. |

### internal/extproc/server.Outcome

| Value | Meaning |
|---|---|
| `authentication_failed` | The broker or the OAuth provider rejected a credential, or ExtProc's own client assertion had expired; the failure detail tells which. |
| `authorization_denied` | The OPA policy, the grant context, the broker or the OAuth provider refused access; the failure detail tells which, and whether ExtProc returned a consent URL. |
| `canceled` | The request was canceled, or its deadline passed, while ExtProc waited for the exchange or sent its response; ExtProc does not set the error status on the span. |
| `configuration_error` | ExtProc's configuration, or the metadata that the gateway sends for policy evaluation, is unusable; fix it, because a retry fails the same way. |
| `infrastructure_error` | A dependency or ExtProc itself failed, such as an unreachable broker, an open circuit breaker or an unavailable grant context; a retry may succeed. |
| `invalid_request` | ExtProc found the request's metadata, method or body malformed, or the broker or the OAuth provider rejected the request it received as invalid. |
| `reauth_required` | The broker answered `invalid_grant` with an `error_uri` for a new sign-in, or an OAuth error had the code `reauth_required`; the failure detail tells whether the client got the URL. |
| `success` | The checks and the exchange passed, or the client assertion was refreshed; a later policy decision on a request body does not change it. |

### internal/extproc/server.RecoveryAction

| Value | Meaning |
|---|---|
| `fix_configuration` | An operator should fix ExtProc's configuration or the gateway's metadata; ExtProc suggests it for `configuration` and `policy_configuration`. |
| `none` | ExtProc suggests no recovery: the request succeeded or was canceled, or no listed action fits, as for rejected credentials and policy denials. |
| `reauthenticate` | The user should sign in again at the `error_uri` that ExtProc returned; ExtProc suggests it only for `session_reauth_required`. |
| `reconsent` | The user should grant consent again at the URL that ExtProc returned; ExtProc suggests it for `consent_required` and `grant_context_denied`. |
| `retry` | The failure may be temporary and a later request may succeed; ExtProc suggests it for every `infrastructure_error` outcome. |

### internal/extproc/server.RecoveryTarget

| Value | Meaning |
|---|---|
| `consent` | The user's consent for the agent, which the user renews at the consent URL that ExtProc returned; it goes with `reconsent`. |
| `none` | The recovery action has no target, which holds for every action except `reconsent` and `reauthenticate`. |
| `provider_session` | The user's provider session that the broker holds, which the user renews at the returned `error_uri`; it goes with `reauthenticate`. |
