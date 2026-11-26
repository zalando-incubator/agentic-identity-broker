CREATE TABLE service_accounts (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES thirdparty_oauth2_services(id) ON DELETE RESTRICT,
    canonical_id VARCHAR(128) NOT NULL,
    kind VARCHAR(64) NOT NULL,
    scopes JSONB NOT NULL,
    credential_encrypted BYTEA NOT NULL,
    credential_summary JSONB NOT NULL,
    credential_expires_at TIMESTAMPTZ,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_service_accounts_service_canonical_id UNIQUE (service_id, canonical_id),
    CONSTRAINT chk_service_accounts_canonical_id CHECK (canonical_id COLLATE "C" ~ '^[A-Za-z0-9._-]{1,128}$'),
    CONSTRAINT chk_service_accounts_kind CHECK (kind <> ''),
    CONSTRAINT chk_service_accounts_scopes CHECK (jsonb_typeof(scopes) = 'array' AND jsonb_array_length(scopes) > 0),
    CONSTRAINT chk_service_accounts_credential_encrypted CHECK (octet_length(credential_encrypted) > 0),
    CONSTRAINT chk_service_accounts_credential_summary CHECK (jsonb_typeof(credential_summary) = 'object'),
    CONSTRAINT chk_service_accounts_version CHECK (version >= 1)
);

COMMENT ON TABLE service_accounts IS 'Machine credentials attached to one third-party service, bounded by a scope ceiling. A service that holds an account cannot be deleted.';
COMMENT ON COLUMN service_accounts.canonical_id IS 'Administrator-chosen name, unique within the service.';
COMMENT ON COLUMN service_accounts.kind IS 'How the account obtains tokens; the accepted set is enforced by the broker.';
COMMENT ON COLUMN service_accounts.scopes IS 'The ceiling: a non-empty JSON array of scope values in ascending byte order.';
COMMENT ON COLUMN service_accounts.credential_encrypted IS 'Envelope-encrypted credential document under the owning service_id context. Ciphertext only; never plaintext.';
COMMENT ON COLUMN service_accounts.credential_summary IS 'Non-secret recognition data: format, use, unverified iss, sub, kid, exp and a SHA-256 fingerprint.';
COMMENT ON COLUMN service_accounts.credential_expires_at IS 'The exp the stored credential states; NULL when it states none. Status is derived from it on read.';
COMMENT ON COLUMN service_accounts.version IS 'Incremented on every committed change; the strong ETag.';
