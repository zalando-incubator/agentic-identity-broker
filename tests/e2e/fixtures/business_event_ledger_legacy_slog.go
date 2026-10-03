package fixtures

// BusinessEventLegacySlogRevision identifies the production code used for this capture.
const BusinessEventLegacySlogRevision = "d500f36378dd914f8a516604a08525f737e8ddff"

// LegacySlogRecord retains log structure, never runtime attribute values.
type LegacySlogRecord struct {
	Event     string
	Level     string
	Message   string
	FieldKeys []string
	Count     int
}

// LegacySlogWorkflow describes one executed journey and its existing fact logs.
// NoEventLine means no dedicated fact log, not absence of generic request logs.
type LegacySlogWorkflow struct {
	Variant          string
	Scenario         string
	Source           string
	AdditionalAction string
	NoEventLine      bool
	Records          []LegacySlogRecord
}

// BusinessEventLegacySlog captures all 28 catalogue facts before the ledger refactor.
// Counts apply to the complete named journey, including repeated business actions.
var BusinessEventLegacySlog = map[string][]LegacySlogWorkflow{
	"agent-deleted": {
		{
			Variant:          "default",
			Scenario:         "Canonical Resource IDs creates a managed resource with canonical and UUID identifiers",
			Source:           "tests/e2e/canonical_resource_ids_test.go:138",
			AdditionalAction: "DELETE the created agent, expect 204, then GET it and expect 404.",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "agent deleted", FieldKeys: []string{"agent_id", "level", "msg", "time"}, Count: 2},
			},
		},
	},
	"agent-registered": {
		{
			Variant:  "default",
			Scenario: "Agent Permission Requirements User Story 1: Administrator Configures Agent Service Requirements should accept optional service_requirements array in POST /api/agents",
			Source:   "tests/e2e/agent_permission_requirements_test.go:177",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "agent created", FieldKeys: []string{"agent_id", "client_id", "level", "msg", "time"}, Count: 1},
			},
		},
	},
	"agent-updated": {
		{
			Variant:  "default",
			Scenario: "Agent Permission Requirements User Story 1: Administrator Configures Agent Service Requirements should replace entire service requirements on PUT /api/agents/{agent-id}",
			Source:   "tests/e2e/agent_permission_requirements_test.go:314",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "agent updated", FieldKeys: []string{"agent_id", "client_id", "level", "msg", "time"}, Count: 1},
			},
		},
	},
	"approval-approved": {
		{
			Variant:  "default",
			Scenario: "Tool Approval API US1: User Approves a Pending Tool Call transitions to approved with persistence=once (US1-S2)",
			Source:   "tests/e2e/approval_api_test.go:274",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "approval approved", FieldKeys: []string{"action", "agent_id", "approval_id", "level", "msg", "persistence", "principal", "time", "timestamp", "tool_name"}, Count: 1},
			},
		},
	},
	"approval-consumed": {
		{
			Variant:  "default",
			Scenario: "Tool Approval API US5: One-Time Approval Consumed After Use marks once-persistence approval as consumed (US5-S1)",
			Source:   "tests/e2e/approval_api_test.go:684",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "approval consumed", FieldKeys: []string{"agent_id", "approval_id", "level", "msg", "principal", "time", "timestamp", "tool_name"}, Count: 1},
			},
		},
	},
	"approval-denied": {
		{
			Variant:  "default",
			Scenario: "Tool Approval API US2: User Denies a Pending Tool Call transitions to denied state (US2-S1)",
			Source:   "tests/e2e/approval_api_test.go:368",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "approval denied", FieldKeys: []string{"action", "agent_id", "approval_id", "level", "msg", "persistence", "principal", "time", "timestamp", "tool_name"}, Count: 1},
			},
		},
	},
	"approval-expired": {
		{
			Variant:  "default",
			Scenario: "Tool Approval API US1: User Approves a Pending Tool Call returns 410 for expired approval (US1-S6)",
			Source:   "tests/e2e/approval_api_test.go:319",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "approval expired (lazy detection)", FieldKeys: []string{"action", "agent_id", "approval_id", "expired_at", "level", "msg", "principal", "time", "timestamp", "tool_name"}, Count: 1},
			},
		},
	},
	"approval-requested": {
		{
			Variant:  "default",
			Scenario: "Tool Approval API US3: ExtProc Creates a Pending Approval creates pending approval with approval_url (US3-S1)",
			Source:   "tests/e2e/approval_api_test.go:429",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "approval created", FieldKeys: []string{"action", "agent_id", "approval_id", "expires_at", "level", "msg", "principal", "time", "timestamp", "tool_name"}, Count: 1},
			},
		},
	},
	"approval-revoked": {
		{
			Variant:  "default",
			Scenario: "ExtProc Approval Journey stops forwarding after a permanent approval is revoked",
			Source:   "tests/e2e/extproc_approval_journey_test.go:140",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "permanent approval revoked", FieldKeys: []string{"agent_id", "approval_id", "level", "msg", "principal", "time", "timestamp", "tool_name"}, Count: 1},
			},
		},
	},
	"authorization-requested": {
		{
			Variant:  "default",
			Scenario: "US4: Authorization Code Flow with PKCE (local mode) full auth code flow with PKCE",
			Source:   "tests/e2e/oauth2_authorize_e2e_test.go:95",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "OAuth2 Authorization Request", FieldKeys: []string{"actor", "duration_ms", "level", "method", "msg", "path", "principal", "remote_host", "request_id", "status", "time", "trace_id", "user_agent"}, Count: 2},
			},
		},
	},
	"credential-generated": {
		{
			Variant:  "default",
			Scenario: "US1: Client Credential Management (local mode) generates credentials for an agent",
			Source:   "tests/e2e/oauth2_client_credentials_e2e_test.go:57",
			Records: []LegacySlogRecord{
				{Event: "CredentialGenerated", Level: "INFO", Message: "CredentialGenerated", FieldKeys: []string{"agent_id", "client_id", "event", "level", "msg", "time"}, Count: 1},
			},
		},
	},
	"credential-revoked": {
		{
			Variant:  "default",
			Scenario: "Canonical Resource IDs manages an agent through canonical ID and UUID",
			Source:   "tests/e2e/canonical_resource_ids_test.go:145",
			Records: []LegacySlogRecord{
				{Event: "CredentialRevoked", Level: "INFO", Message: "CredentialRevoked", FieldKeys: []string{"agent_id", "event", "level", "msg", "time"}, Count: 1},
			},
		},
	},
	"credential-rotated": {
		{
			Variant:  "default",
			Scenario: "US1: Client Credential Management (local mode) rotates existing credentials",
			Source:   "tests/e2e/oauth2_client_credentials_e2e_test.go:74",
			Records: []LegacySlogRecord{
				{Event: "CredentialRotated", Level: "INFO", Message: "CredentialRotated", FieldKeys: []string{"agent_id", "client_id", "event", "level", "msg", "time"}, Count: 1},
			},
		},
	},
	"grant-created": {
		{
			Variant:  "default",
			Scenario: "Permission Sets (019) US4: User Grant Stores Granted Permission Sets stores granted_permission_sets in UserGrant on consent submission",
			Source:   "tests/e2e/permission_sets_test.go:1065",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "grant created", FieldKeys: []string{"actor", "agent_id", "grant_id", "level", "msg", "principal", "time", "trace_id"}, Count: 1},
			},
		},
	},
	"grant-expired": {
		{
			Variant:     "default",
			Scenario:    "OAuth2 Authorization Endpoint when user's grant has expired should treat expired grant as non-existent and redirect to consent UI",
			Source:      "tests/e2e/oauth2_authorize_test.go:241",
			NoEventLine: true,
		},
	},
	"grant-revoked": {
		{
			Variant:  "default",
			Scenario: "Revoke Agent Grant returns 204 when grant exists and is owned by principal",
			Source:   "tests/e2e/revoke_grant_test.go:111",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "grant revoked", FieldKeys: []string{"action", "agent_id", "grant_id", "level", "msg", "principal", "time"}, Count: 1},
			},
		},
	},
	"grant-updated": {
		{
			Variant:  "default",
			Scenario: "Permission Sets (019) US4: User Grant Stores Granted Permission Sets upserts UserGrant with new granted_permission_sets on re-consent",
			Source:   "tests/e2e/permission_sets_test.go:1193",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "grant created", FieldKeys: []string{"actor", "agent_id", "grant_id", "level", "msg", "principal", "time", "trace_id"}, Count: 2},
			},
		},
	},
	"impersonation-denied": {
		{
			Variant:  "default",
			Scenario: "OAuth2 User Impersonation local mode with a matching signed rule returns access_denied when no rule predicate permits the client",
			Source:   "tests/e2e/impersonation_test.go:498",
			Records: []LegacySlogRecord{
				{Event: "impersonation_decision", Level: "INFO", Message: "impersonation_decision", FieldKeys: []string{"actor", "actor_identity", "audience", "event", "failure_category", "issuer_identifiers", "issuer_roles", "level", "msg", "oauth_error_code", "outcome", "privileged_client_identity", "request_id", "rule", "subject_identity", "target_agent_id", "time", "trace_id"}, Count: 1},
			},
		},
	},
	"impersonation-granted": {
		{
			Variant:  "default",
			Scenario: "OAuth2 User Impersonation local mode with a matching signed rule emits a credential-free audit event on success",
			Source:   "tests/e2e/impersonation_test.go:524",
			Records: []LegacySlogRecord{
				{Event: "impersonation_decision", Level: "INFO", Message: "impersonation_decision", FieldKeys: []string{"actor", "actor_identity", "audience", "event", "issuer_identifiers", "issuer_roles", "level", "msg", "outcome", "privileged_client_identity", "request_id", "rule", "subject_identity", "target_agent_id", "time", "trace_id"}, Count: 1},
			},
		},
	},
	"session-established": {
		{
			Variant:  "default",
			Scenario: "Public Client Support for Third-Party OAuth2 Services User Story 2: authorize a user against a public-client provider stores an encrypted session exactly as for a confidential service",
			Source:   "tests/e2e/thirdparty_public_client_test.go:323",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "OAuth2 callback successful", FieldKeys: []string{"level", "msg", "principal", "service_id", "session_id", "time"}, Count: 1},
				{Event: "", Level: "INFO", Message: "session created", FieldKeys: []string{"level", "msg", "principal", "service_id", "session_id", "time"}, Count: 1},
				{Event: "session.oauth2.session_established", Level: "INFO", Message: "oauth2_session_established", FieldKeys: []string{"event", "level", "msg", "principal", "public_client", "service_id", "session_id", "time", "timestamp"}, Count: 1},
			},
		},
	},
	"session-refresh-failed": {
		{
			Variant:  "default",
			Scenario: "Public Client Support for Third-Party OAuth2 Services User Story 3: keep a public-client session alive surfaces the failure without retrying with a credential or downgrading",
			Source:   "tests/e2e/thirdparty_public_client_test.go:507",
			Records: []LegacySlogRecord{
				{Event: "", Level: "ERROR", Message: "force refresh failed at upstream", FieldKeys: []string{"error", "level", "msg", "principal", "service_id", "time"}, Count: 1},
				{Event: "session.oauth2.refresh_failed", Level: "ERROR", Message: "oauth2_refresh_failed", FieldKeys: []string{"error", "event", "level", "msg", "principal", "public_client", "reason", "service_id", "time", "timestamp"}, Count: 1},
			},
		},
	},
	"session-refreshed": {
		{
			Variant:  "default",
			Scenario: "Public Client Support for Third-Party OAuth2 Services User Story 3: keep a public-client session alive encrypts and persists the refreshed tokens, replacing the previous ones",
			Source:   "tests/e2e/thirdparty_public_client_test.go:457",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "access token refreshed", FieldKeys: []string{"level", "msg", "service_id", "time", "token_endpoint"}, Count: 2},
				{Event: "", Level: "INFO", Message: "session tokens updated", FieldKeys: []string{"level", "msg", "principal", "service_id", "session_id", "time"}, Count: 2},
				{Event: "session.oauth2.token_refreshed", Level: "INFO", Message: "oauth2_token_refreshed", FieldKeys: []string{"event", "level", "msg", "principal", "public_client", "reason", "service_id", "time", "timestamp"}, Count: 2},
			},
		},
	},
	"session-terminated": {
		{
			Variant:          "default",
			Scenario:         "Public Client Support for Third-Party OAuth2 Services User Story 2: authorize a user against a public-client provider stores an encrypted session exactly as for a confidential service",
			Source:           "tests/e2e/thirdparty_public_client_test.go:323",
			AdditionalAction: "DELETE the established session, expect 200, then GET it and expect 404.",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "session terminated successfully", FieldKeys: []string{"level", "msg", "principal", "service_id", "time"}, Count: 1},
				{Event: "session.oauth2.session_terminated", Level: "INFO", Message: "oauth2_session_terminated", FieldKeys: []string{"event", "initiated_at", "level", "msg", "principal", "service_id", "session_id", "time", "timestamp"}, Count: 1},
				{Event: "session.oauth2.termination_initiated", Level: "INFO", Message: "oauth2_session_termination_initiated", FieldKeys: []string{"event", "level", "msg", "principal", "service_id", "time", "timestamp"}, Count: 1},
			},
		},
	},
	"signing-key-promoted": {
		{
			Variant:  "default",
			Scenario: "US5: Signing Key Management (local mode) promotes a key to current",
			Source:   "tests/e2e/oauth2_signing_keys_e2e_test.go:198",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "signing key promoted to current", FieldKeys: []string{"kid", "level", "msg", "operator_principal", "time"}, Count: 1},
			},
		},
	},
	"token-exchange-denied": {
		{
			Variant:  "default",
			Scenario: "RFC 8693 Token Exchange E2E Tests US1: Gateway Exchanges Token for Third-Party Token [US1-S5] should return 401 invalid_client without valid client_assertion",
			Source:   "tests/e2e/token_exchange_test.go:275",
			Records: []LegacySlogRecord{
				{Event: "", Level: "ERROR", Message: "Token exchange failed", FieldKeys: []string{"actor", "cause", "error", "error_type", "level", "msg", "resource", "time", "trace_id"}, Count: 1},
			},
		},
	},
	"token-exchanged": {
		{
			Variant:  "default",
			Scenario: "RFC 8693 Token Exchange E2E Tests US1: Gateway Exchanges Token for Third-Party Token [US1-S3] should return RFC 8693 response with access_token and token_type",
			Source:   "tests/e2e/token_exchange_test.go:209",
			Records: []LegacySlogRecord{
				{Event: "", Level: "INFO", Message: "token_exchange_succeeded", FieldKeys: []string{"actor", "calling_peer", "issued_token_type", "level", "msg", "resource", "time", "trace_id"}, Count: 1},
			},
		},
	},
	"token-issued": {
		{
			Variant:  "local-client-credentials",
			Scenario: "US3: Client Credentials Grant (local mode) client_credentials grant issues signed token",
			Source:   "tests/e2e/oauth2_token_e2e_test.go:82",
			Records: []LegacySlogRecord{
				{Event: "TokenIssued", Level: "INFO", Message: "TokenIssued", FieldKeys: []string{"actor", "client_id", "event", "grant_type", "level", "msg", "scope", "time", "trace_id"}, Count: 1},
			},
		},
		{
			Variant:     "proxy",
			Scenario:    "OAuth2 Token Endpoint E2E (User Story 2) Scenario 1: Successful authorization code exchange should exchange authorization code for tokens using grant_type=authorization_code",
			Source:      "tests/e2e/oauth2_token_test.go:85",
			NoEventLine: true,
		},
		{
			Variant:  "hybrid",
			Scenario: "US2: Hybrid Mode — Local Agent Full Authorization Code Journey local agent in hybrid mode issues JWT via full authorization code flow with PKCE",
			Source:   "tests/e2e/hybrid_oauth_modes_e2e_test.go:440",
			Records: []LegacySlogRecord{
				{Event: "TokenIssued", Level: "INFO", Message: "TokenIssued", FieldKeys: []string{"actor", "client_id", "event", "grant_type", "level", "msg", "time", "trace_id"}, Count: 1},
			},
		},
		{
			Variant:     "hybrid-proxy",
			Scenario:    "US2: Hybrid Mode — Proxy Agent Full Authorization Code Journey proxy agent in hybrid mode forwards authorize to upstream and proxies token response",
			Source:      "tests/e2e/hybrid_oauth_modes_e2e_test.go:639",
			NoEventLine: true,
		},
	},
	"token-request-failed": {
		{
			Variant:  "default",
			Scenario: "US3: Client Credentials Grant (local mode) invalid credentials returns 401",
			Source:   "tests/e2e/oauth2_token_e2e_test.go:104",
			Records: []LegacySlogRecord{
				{Event: "", Level: "ERROR", Message: "client_credentials grant failed", FieldKeys: []string{"actor", "client_id", "error", "level", "msg", "time", "trace_id"}, Count: 1},
				{Event: "TokenRequestFailed", Level: "WARN", Message: "TokenRequestFailed", FieldKeys: []string{"actor", "client_id", "error_code", "error_description", "event", "grant_type", "level", "msg", "time", "trace_id"}, Count: 1},
			},
		},
	},
}
