CREATE TABLE refresh_sessions (
    id UUID PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'::uuid),
    -- The grant can be deleted, but its original ID must remain as authorization evidence.
    original_grant_id UUID NOT NULL CHECK (original_grant_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    original_token_signature TEXT NOT NULL CHECK (original_token_signature ~ '^[0-9a-f]{64}$'),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    principal TEXT NOT NULL CHECK (principal <> ''),
    client_id TEXT NOT NULL CHECK (client_id <> ''),
    scope TEXT NOT NULL,
    email TEXT,
    display_name TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL CHECK (isfinite(started_at)),
    last_fresh_at TIMESTAMPTZ NOT NULL CHECK (isfinite(last_fresh_at)),
    absolute_expires_at TIMESTAMPTZ CHECK (isfinite(absolute_expires_at)),
    inactivity_expires_at TIMESTAMPTZ NOT NULL CHECK (isfinite(inactivity_expires_at)),
    retain_until TIMESTAMPTZ NOT NULL CHECK (isfinite(retain_until)),
    branch_key_id TEXT NOT NULL CHECK (branch_key_id = 'refresh_' || id::text || '_branch_key'),
    current_signature TEXT NOT NULL CHECK (current_signature ~ '^[0-9a-f]{64}$'),
    previous_signature TEXT CHECK (previous_signature ~ '^[0-9a-f]{64}$'),
    previous_consumed_at TIMESTAMPTZ CHECK (isfinite(previous_consumed_at)),
    reuse_until TIMESTAMPTZ CHECK (isfinite(reuse_until)),
    original_requested_scope TEXT,
    original_request_context_fingerprint TEXT CHECK (original_request_context_fingerprint ~ '^[0-9a-f]{64}$'),
    retry_ciphertext BYTEA,
    retry_access_expires_at TIMESTAMPTZ CHECK (isfinite(retry_access_expires_at)),
    retry_expires_at TIMESTAMPTZ CHECK (isfinite(retry_expires_at)),
    retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count BETWEEN 0 AND 3),
    revoked_at TIMESTAMPTZ CHECK (isfinite(revoked_at)),
    expired_at TIMESTAMPTZ CHECK (isfinite(expired_at)),
    terminal_reason TEXT,
    CONSTRAINT refresh_sessions_clocks CHECK (
        last_fresh_at >= started_at
        AND inactivity_expires_at > last_fresh_at
        AND retain_until > started_at
        AND (absolute_expires_at IS NULL OR absolute_expires_at > started_at)
    ),
    CONSTRAINT refresh_sessions_predecessor CHECK (
        (previous_signature IS NULL
            AND previous_consumed_at IS NULL AND reuse_until IS NULL
            AND original_requested_scope IS NULL AND original_request_context_fingerprint IS NULL
            AND retry_access_expires_at IS NULL AND retry_expires_at IS NULL
            AND retry_ciphertext IS NULL AND retry_count = 0
            AND current_signature = original_token_signature AND last_fresh_at = started_at)
        OR
        (previous_signature IS NOT NULL
            AND previous_consumed_at IS NOT NULL AND reuse_until IS NOT NULL
            AND original_requested_scope IS NOT NULL AND original_request_context_fingerprint IS NOT NULL
            AND retry_access_expires_at IS NOT NULL AND retry_expires_at IS NOT NULL
            AND current_signature <> previous_signature
            AND previous_consumed_at = last_fresh_at AND previous_consumed_at >= started_at
            AND reuse_until >= previous_consumed_at
            AND retry_access_expires_at > started_at
            AND retry_expires_at <= reuse_until AND retry_expires_at <= retry_access_expires_at
            AND retry_expires_at <= inactivity_expires_at
            AND (absolute_expires_at IS NULL OR retry_expires_at <= absolute_expires_at)
            AND (reuse_until > previous_consumed_at OR retry_ciphertext IS NULL))
    ),
    CONSTRAINT refresh_sessions_terminal CHECK (
        (revoked_at IS NULL AND expired_at IS NULL AND terminal_reason IS NULL)
        OR
        (revoked_at IS NOT NULL AND revoked_at >= started_at AND expired_at IS NULL AND retry_ciphertext IS NULL AND terminal_reason IS NOT NULL
            AND terminal_reason IN ('grant_deleted', 'expired_grant_renewal', 'agent_deleted',
                'credential_revoked', 'code_replay', 'prohibited_reuse', 'restore_invalidation'))
        OR
        (expired_at IS NOT NULL AND expired_at >= started_at AND revoked_at IS NULL AND retry_ciphertext IS NULL AND terminal_reason IS NOT NULL
            AND terminal_reason IN ('absolute_expiry', 'inactivity_expiry'))
    )
);

CREATE TABLE refresh_tokens (
    signature TEXT PRIMARY KEY CHECK (signature ~ '^[0-9a-f]{64}$'),
    session_id UUID NOT NULL REFERENCES refresh_sessions(id) ON DELETE CASCADE,
    issued_at TIMESTAMPTZ NOT NULL CHECK (isfinite(issued_at)),
    expires_at TIMESTAMPTZ NOT NULL CHECK (isfinite(expires_at)),
    used_at TIMESTAMPTZ CHECK (isfinite(used_at)),
    CONSTRAINT refresh_tokens_clocks CHECK (
        expires_at > issued_at AND (used_at IS NULL OR used_at >= issued_at)
    ),
    CONSTRAINT refresh_tokens_lineage UNIQUE (session_id, signature, issued_at)
);

-- The root precedes its first child in an issuance transaction. Both references
-- are checked at commit so root, token and legacy mirror can be written together.
ALTER TABLE refresh_sessions
    ADD CONSTRAINT refresh_sessions_original_token_fk
        FOREIGN KEY (id, original_token_signature, started_at)
        REFERENCES refresh_tokens(session_id, signature, issued_at)
        DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT refresh_sessions_current_token_fk
        FOREIGN KEY (id, current_signature, last_fresh_at)
        REFERENCES refresh_tokens(session_id, signature, issued_at)
        DEFERRABLE INITIALLY DEFERRED;

CREATE UNIQUE INDEX idx_refresh_tokens_one_unused_per_session
    ON refresh_tokens (session_id) WHERE used_at IS NULL;
CREATE INDEX idx_refresh_sessions_agent_principal ON refresh_sessions (agent_id, principal);
CREATE INDEX idx_refresh_sessions_inactivity_due ON refresh_sessions (inactivity_expires_at)
    WHERE terminal_reason IS NULL;
CREATE INDEX idx_refresh_sessions_absolute_due ON refresh_sessions (absolute_expires_at)
    WHERE terminal_reason IS NULL AND absolute_expires_at IS NOT NULL;
CREATE INDEX idx_refresh_sessions_retry_due ON refresh_sessions (retry_expires_at)
    WHERE retry_ciphertext IS NOT NULL;
CREATE INDEX idx_refresh_sessions_retention ON refresh_sessions (retain_until)
    WHERE terminal_reason IS NOT NULL;

ALTER TABLE refresh_token_sessions
    ADD COLUMN session_id UUID REFERENCES refresh_sessions(id) ON DELETE CASCADE,
    ADD COLUMN predecessor_signature TEXT,
    ADD CONSTRAINT refresh_token_sessions_predecessor_digest CHECK (
        predecessor_signature IS NULL OR
        (session_id IS NOT NULL AND predecessor_signature ~ '^[0-9a-f]{64}$')
    );

CREATE INDEX idx_refresh_token_sessions_session_id ON refresh_token_sessions (session_id)
    WHERE session_id IS NOT NULL;
CREATE INDEX idx_refresh_token_sessions_agent_request_id ON refresh_token_sessions (agent_id, request_id)
    WHERE used_at IS NULL;

-- Audit history must outlive deletion of either the agent or the refresh root.
CREATE TABLE refresh_revocation_receipts (
    session_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    principal TEXT NOT NULL CHECK (principal <> ''),
    client_id TEXT NOT NULL CHECK (client_id <> ''),
    reason TEXT NOT NULL CHECK (reason IN (
        'grant_deleted', 'expired_grant_renewal', 'agent_deleted', 'credential_revoked',
        'code_replay', 'prohibited_reuse', 'absolute_expiry', 'inactivity_expiry',
        'restore_invalidation'
    )),
    "at" TIMESTAMPTZ NOT NULL CHECK (isfinite("at")),
    redacted_context JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(redacted_context) = 'object'),
    PRIMARY KEY (session_id, reason)
);

CREATE FUNCTION reject_refresh_revocation_receipt_change() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'refresh revocation receipts are immutable';
END;
$$;

CREATE TRIGGER refresh_revocation_receipts_immutable
    BEFORE UPDATE OR DELETE ON refresh_revocation_receipts
    FOR EACH ROW EXECUTE FUNCTION reject_refresh_revocation_receipt_change();
