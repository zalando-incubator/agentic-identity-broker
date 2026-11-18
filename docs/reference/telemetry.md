---
title: "Telemetry"
description: Spans, metrics and attributes that the broker and ExtProc emit through OpenTelemetry. Generated from the source code.
---

# Telemetry

This page lists every span, metric and attribute that the broker and ExtProc create through
the OpenTelemetry API. `tools/telemetrydoc` generates it from the source code, and a test
fails when the page is out of date. Do not edit it by hand. After you change a span or a
metric, run `just telemetry-doc`.

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

| Span | Kind | Scope | Error status | Started in |
|---|---|---|---|---|
| [`approval.approve`](#approvalapprove) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`approval.consume`](#approvalconsume) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`approval.created`](#approvalcreated) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`approval.deny`](#approvaldeny) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`approval.list_pending`](#approvallist_pending) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`approval.list_permanent`](#approvallist_permanent) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`approval.review`](#approvalreview) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`approval.revoke`](#approvalrevoke) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`approval.sync`](#approvalsync) | Internal | `approval` | No | `internal/domain/approval/service.go` |
| [`encryption.decrypt`](#encryptiondecrypt) | Internal | `encryption` | No | `internal/adapters/encryption/aws/adapter.go` |
| [`encryption.encrypt`](#encryptionencrypt) | Internal | `encryption` | No | `internal/adapters/encryption/aws/adapter.go` |
| [`extproc.approval.gate`](#extprocapprovalgate) | Internal | `extproc` | No | `internal/extproc/approval/gate.go` |
| [`extproc.client_assertion.refresh`](#extprocclient_assertionrefresh) | Internal | `extproc` | Yes | `internal/extproc/server/exchanger.go` |
| [`extproc.opa.evaluate`](#extprocopaevaluate) | Internal | `extproc` | No | `internal/extproc/authorization/authorizer.go` |
| [`extproc.token_exchange`](#extproctoken_exchange) | Server | `extproc` | Yes | `internal/extproc/server/server.go` |
| [`http.request`](#httprequest) | Client | `telemetryhttp` | Yes | `internal/telemetryhttp/transport.go` |
| [`jwks.fetch`](#jwksfetch) | Internal | `jwks` | Yes | `internal/adapters/jwks/adapter.go` |
| [`oauth2.token_proxy`](#oauth2token_proxy) | Internal | `upstream` | No | `internal/adapters/http/enduser/token_grant_strategy.go` |
| [`oauth2session.background_refresh`](#oauth2sessionbackground_refresh) | Internal | `oauth2session` | No | `internal/domain/oauth2session/background_refresh.go` |
| [`storage.approve.tool_approval`](#storageapprovetool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.consume.client_assertion_replay`](#storageconsumeclient_assertion_replay) | Client | `storage` | Yes | `internal/adapters/storage/postgres/client_assertion_replay_repo.go` |
| [`storage.consume.tool_approval`](#storageconsumetool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.count_pending.tool_approval`](#storagecount_pendingtool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.create.authorization_code`](#storagecreateauthorization_code) | Client | `storage` | Yes | `internal/adapters/storage/postgres/authorization_code_repo.go` |
| [`storage.create.client_credential`](#storagecreateclient_credential) | Client | `storage` | Yes | `internal/adapters/storage/postgres/client_credential_repo.go` |
| [`storage.create.pkce_session`](#storagecreatepkce_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/pkce_session_repo.go` |
| [`storage.create.refresh_token_session`](#storagecreaterefresh_token_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/refresh_token_session_repo.go` |
| [`storage.create.tool_approval`](#storagecreatetool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.create.user_grant`](#storagecreateuser_grant) | Client | `storage` | Yes | `internal/adapters/storage/postgres/user_grants.go` |
| [`storage.delete.client_credential`](#storagedeleteclient_credential) | Client | `storage` | Yes | `internal/adapters/storage/postgres/client_credential_repo.go` |
| [`storage.delete.pkce_session`](#storagedeletepkce_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/pkce_session_repo.go` |
| [`storage.delete_expired.authorization_code`](#storagedelete_expiredauthorization_code) | Client | `storage` | Yes | `internal/adapters/storage/postgres/authorization_code_repo.go` |
| [`storage.delete_expired.client_assertion_replay`](#storagedelete_expiredclient_assertion_replay) | Client | `storage` | Yes | `internal/adapters/storage/postgres/client_assertion_replay_repo.go` |
| [`storage.delete_expired.pkce_session`](#storagedelete_expiredpkce_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/pkce_session_repo.go` |
| [`storage.delete_expired.refresh_token_session`](#storagedelete_expiredrefresh_token_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/refresh_token_session_repo.go` |
| [`storage.deny.tool_approval`](#storagedenytool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.find.authorization_code`](#storagefindauthorization_code) | Client | `storage` | Yes | `internal/adapters/storage/postgres/authorization_code_repo.go` |
| [`storage.find.pkce_session`](#storagefindpkce_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/pkce_session_repo.go` |
| [`storage.find.refresh_token_session`](#storagefindrefresh_token_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/refresh_token_session_repo.go` |
| [`storage.get.agent`](#storagegetagent) | Client | `storage` | Yes | `internal/adapters/storage/postgres/agent_repository.go` |
| [`storage.get.client_credential`](#storagegetclient_credential) | Client | `storage` | Yes | `internal/adapters/storage/postgres/client_credential_repo.go` |
| [`storage.get.tool_approval`](#storagegettool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.get_version.approval_sync`](#storageget_versionapproval_sync) | Client | `storage` | Yes | `internal/adapters/storage/postgres/approval_sync_state_repository.go` |
| [`storage.increment_version.approval_sync`](#storageincrement_versionapproval_sync) | Client | `storage` | Yes | `internal/adapters/storage/postgres/approval_sync_state_repository.go` |
| [`storage.list.user_grants_by_principal`](#storagelistuser_grants_by_principal) | Client | `storage` | Yes | `internal/adapters/storage/postgres/user_grants.go` |
| [`storage.list_active.tool_approval`](#storagelist_activetool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.list_permanent.tool_approval`](#storagelist_permanenttool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.mark_used.authorization_code`](#storagemark_usedauthorization_code) | Client | `storage` | Yes | `internal/adapters/storage/postgres/authorization_code_repo.go` |
| [`storage.mark_used.refresh_token_session`](#storagemark_usedrefresh_token_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/refresh_token_session_repo.go` |
| [`storage.revoke_by_agent.refresh_token_session`](#storagerevoke_by_agentrefresh_token_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/refresh_token_session_repo.go` |
| [`storage.revoke_by_principal_and_agent.refresh_token_session`](#storagerevoke_by_principal_and_agentrefresh_token_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/refresh_token_session_repo.go` |
| [`storage.revoke_by_request.refresh_token_session`](#storagerevoke_by_requestrefresh_token_session) | Client | `storage` | Yes | `internal/adapters/storage/postgres/refresh_token_session_repo.go` |
| [`storage.revoke_permanent.tool_approval`](#storagerevoke_permanenttool_approval) | Client | `storage` | Yes | `internal/adapters/storage/postgres/tool_approval_repository.go` |
| [`storage.rotate.client_credential`](#storagerotateclient_credential) | Client | `storage` | Yes | `internal/adapters/storage/postgres/client_credential_repo.go` |
| [`tokenexchange.exchange`](#tokenexchangeexchange) | Internal | `tokenexchange` | Yes | `internal/adapters/http/enduser/oauth2_token.go` |
| [`tokenexchange.get_valid_access_token`](#tokenexchangeget_valid_access_token) | Internal | `tokenexchange` | No | `internal/domain/tokenexchange/service.go` |
| [`tokenexchange.impersonation`](#tokenexchangeimpersonation) | Internal | `tokenexchange` | Yes | Renamed from `tokenexchange.exchange` |
| [`tokenexchange.verify_agent_access`](#tokenexchangeverify_agent_access) | Internal | `tokenexchange` | No | `internal/domain/tokenexchange/service.go` |

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

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.persistence` | [`internal/domain/storage.ApprovalPersistence`](#internaldomainstorageapprovalpersistence) |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |
| `approval.tool_pattern` | any `string` |

### approval.consume

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.tool_name` | any `string` |

### approval.created

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.is_new` | any `bool` |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |

### approval.deny

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.principal` | any `string` |
| `approval.tool_name` | any `string` |

### approval.list_pending

| Attribute | Values |
|---|---|
| `approval.count` | any `int` |
| `approval.principal` | any `string` |

### approval.list_permanent

| Attribute | Values |
|---|---|
| `approval.count` | any `int` |
| `approval.principal` | any `string` |

### approval.review

| Attribute | Values |
|---|---|
| `approval.id` | any `string` |
| `approval.principal` | any `string` |
| `approval.status` | [`internal/domain/storage.ApprovalStatus`](#internaldomainstorageapprovalstatus) |
| `approval.tool_name` | any `string` |

### approval.revoke

| Attribute | Values |
|---|---|
| `approval.agent_id` | any `string` |
| `approval.id` | any `string` |
| `approval.tool_name` | any `string` |

### approval.sync

| Attribute | Values |
|---|---|
| `approval.pair_count` | any `int` |
| `approval.sync_version` | any `int64` |

### encryption.decrypt

| Attribute | Values |
|---|---|
| `encryption.key_type` | `aws_kms` |

### encryption.encrypt

| Attribute | Values |
|---|---|
| `encryption.key_type` | `aws_kms` |

### extproc.approval.gate

| Attribute | Values |
|---|---|
| `approval.outcome` | `deny`, `elicit`, `proceed` |

### extproc.client_assertion.refresh

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

### extproc.opa.evaluate

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

| Attribute | Values |
|---|---|
| `error.type` | `network_error`, `response_close_failed`, `response_read_failed` |
| `http.request.method` | any `string` |
| `http.response.status_code` | any `int` |

### jwks.fetch

| Attribute | Values |
|---|---|
| `error.type` | `jwks_unavailable` |

### oauth2.token_proxy

| Attribute | Values |
|---|---|
| `http.request.method` | `POST` |
| `http.response.status_code` | any `int` |

### oauth2session.background_refresh

| Attribute | Values |
|---|---|
| `oauth2.refresh.outcome` | `failed`, `not_due`, `refreshed` |
| `oauth2.refresh.triggered_by` | `background` |

### storage.approve.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `ApproveToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.consume.client_assertion_replay

| Attribute | Values |
|---|---|
| `db.operation` | `ClientAssertionReplayRepo.Consume` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.consume.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `ConsumeToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.count_pending.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `CountPendingToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.authorization_code

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.client_credential

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.pkce_session

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.refresh_token_session

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.Create` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `CreateToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.create.user_grant

| Attribute | Values |
|---|---|
| `db.operation` | `CreateUserGrant` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete.client_credential

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Delete` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete.pkce_session

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.Delete` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete_expired.authorization_code

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete_expired.client_assertion_replay

| Attribute | Values |
|---|---|
| `db.operation` | `ClientAssertionReplayRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete_expired.pkce_session

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.delete_expired.refresh_token_session

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.DeleteExpired` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.deny.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `DenyToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.find.authorization_code

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.FindByCodeHash` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.find.pkce_session

| Attribute | Values |
|---|---|
| `db.operation` | `PKCESessionRepo.FindBySignature` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.find.refresh_token_session

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.FindBySignature` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.get.agent

| Attribute | Values |
|---|---|
| `db.operation` | `GetAgent` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.get.client_credential

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.GetByAgentID` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.get.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `GetToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.get_version.approval_sync

| Attribute | Values |
|---|---|
| `db.operation` | `GetApprovalSyncVersion` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.increment_version.approval_sync

| Attribute | Values |
|---|---|
| `db.operation` | `IncrementApprovalSyncVersion` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.list.user_grants_by_principal

| Attribute | Values |
|---|---|
| `db.operation` | `ListByPrincipal` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.list_active.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `ListAllActiveToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.list_permanent.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `ListPermanentToolApprovals` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.mark_used.authorization_code

| Attribute | Values |
|---|---|
| `db.operation` | `AuthorizationCodeRepo.MarkUsed` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.mark_used.refresh_token_session

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.MarkUsed` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.revoke_by_agent.refresh_token_session

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByAgent` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.revoke_by_principal_and_agent.refresh_token_session

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByPrincipalAndAgent` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.revoke_by_request.refresh_token_session

| Attribute | Values |
|---|---|
| `db.operation` | `RefreshTokenSessionRepo.RevokeByRequestID` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.revoke_permanent.tool_approval

| Attribute | Values |
|---|---|
| `db.operation` | `RevokePermanentToolApproval` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### storage.rotate.client_credential

| Attribute | Values |
|---|---|
| `db.operation` | `ClientCredentialRepo.Rotate` |
| `db.system` | `postgresql` |
| `error.type` | [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |
| `storage.outcome` | `canceled`, [`internal/domain/storage.ErrorKind`](#internaldomainstorageerrorkind) |

### tokenexchange.exchange

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

No attributes.

### tokenexchange.impersonation

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

No attributes.

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

`authorization_denied`, `broker_response_invalid`, `broker_unavailable`, `caller_canceled`, `circuit_open`, `client_expired`, `client_invalid`, `configuration`, `consent_required`, `credential_rejected`, `internal_unclassified`, `none`, `policy_configuration`, `policy_denied`, `policy_evaluation_failed`, `provider_response_invalid`, `provider_unavailable`, `reauth_required`, `request_malformed`, `resource_missing`, `response_write_failed`, `session_reauth_required`, `subject_invalid`

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
