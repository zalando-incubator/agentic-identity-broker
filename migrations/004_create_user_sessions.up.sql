-- Migration: 004_create_user_sessions.up.sql
-- Purpose: Create user_sessions table for storing OAuth2 session data
-- Feature: 20251222-120000-thirdparty-oauth2-sessions
-- Date: 2025-12-23

-- Create user_sessions table
-- This table stores OAuth2 sessions between users (principals) and third-party services.
-- Each session contains encrypted access/refresh tokens and session metadata.
CREATE TABLE user_sessions (
    -- Primary key
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- User identification (from X-Remote-User header)
    principal VARCHAR(200) NOT NULL,

    -- Foreign key to thirdparty_oauth2_services
    -- ON DELETE RESTRICT prevents service deletion when active sessions exist
    service_id UUID NOT NULL,

    -- Encrypted tokens (stored as ciphertext)
    -- Access token is required, refresh token is optional
    encrypted_access_token BYTEA NOT NULL,
    encrypted_refresh_token BYTEA,

    -- Token metadata
    token_type VARCHAR(50) NOT NULL DEFAULT 'Bearer',
    access_token_expires_at TIMESTAMPTZ,
    refresh_token_expires_at TIMESTAMPTZ,

    -- OAuth2 scopes granted in this session
    scope TEXT[] NOT NULL DEFAULT '{}',

    -- Encryption context (AAD for AES-GCM)
    -- Contains principal, service_id, session_id, purpose
    -- Used for auditing and prevents cross-context token usage
    encryption_context JSONB NOT NULL DEFAULT '{}',

    -- Session lifecycle timestamps
    initiated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Foreign key constraint with deletion protection
    -- Cannot delete a service that has active sessions (FR-022 pattern)
    CONSTRAINT fk_user_sessions_service
        FOREIGN KEY (service_id)
        REFERENCES thirdparty_oauth2_services(id)
        ON DELETE RESTRICT,

    -- Unique constraint: one session per user per service
    -- Enforces Invariant #1: One Session Per User-Service Pair
    -- Prevents race conditions during OAuth2 callback (first callback wins)
    CONSTRAINT user_sessions_principal_service_unique
        UNIQUE (principal, service_id)
);

-- Indexes for performance

-- Index for fast lookups by principal (list user's sessions)
-- Used by: GET /api/third-party/sessions
CREATE INDEX idx_user_sessions_principal ON user_sessions(principal);

-- Index for counting sessions by service (deletion protection)
-- Used by: ThirdpartyOAuth2ServiceRepository.CountGrantsReferencingService
CREATE INDEX idx_user_sessions_service_id ON user_sessions(service_id);

-- Index for finding sessions by principal and service
-- Covers the unique constraint, improves lookup performance
CREATE INDEX idx_user_sessions_principal_service ON user_sessions(principal, service_id);

-- Trigger to automatically update updated_at timestamp
CREATE OR REPLACE FUNCTION update_user_sessions_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_user_sessions_updated_at
    BEFORE UPDATE ON user_sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_user_sessions_updated_at();

-- Comments for documentation
COMMENT ON TABLE user_sessions IS 'Stores OAuth2 sessions between users and third-party services. Tokens are encrypted at rest using AES-GCM with encryption context binding.';
COMMENT ON COLUMN user_sessions.principal IS 'Authenticated user identifier (email, username, UUID) from X-Remote-User header. Maximum 200 characters.';
COMMENT ON COLUMN user_sessions.service_id IS 'Reference to thirdparty_oauth2_services table. ON DELETE RESTRICT prevents service deletion with active sessions.';
COMMENT ON COLUMN user_sessions.encrypted_access_token IS 'AES-GCM encrypted OAuth2 access token. Ciphertext stored as BYTEA. Decryption requires matching encryption_context.';
COMMENT ON COLUMN user_sessions.encrypted_refresh_token IS 'AES-GCM encrypted OAuth2 refresh token (nullable). Used for token rotation without re-authentication.';
COMMENT ON COLUMN user_sessions.token_type IS 'OAuth2 token type. Usually "Bearer". Default: Bearer.';
COMMENT ON COLUMN user_sessions.access_token_expires_at IS 'Access token expiration timestamp. Null if provider does not return expiration. Future auto-refresh will use this.';
COMMENT ON COLUMN user_sessions.refresh_token_expires_at IS 'Refresh token expiration timestamp. Null if token never expires. Session considered expired when this timestamp passes.';
COMMENT ON COLUMN user_sessions.scope IS 'Array of OAuth2 scopes granted in this session. Example: ["repo", "user:email"].';
COMMENT ON COLUMN user_sessions.encryption_context IS 'JSONB containing encryption context (AAD). Structure: {principal, service_id, session_id, purpose}. Binds ciphertext to session.';
COMMENT ON COLUMN user_sessions.initiated_at IS 'Timestamp when OAuth2 flow was completed and session established. Immutable after creation.';
COMMENT ON COLUMN user_sessions.created_at IS 'Timestamp when record was created. Immutable.';
COMMENT ON COLUMN user_sessions.updated_at IS 'Timestamp when record was last updated. Automatically updated by trigger.';

COMMENT ON CONSTRAINT user_sessions_principal_service_unique ON user_sessions IS
'Enforces one session per (principal, service_id) pair. Prevents race conditions during OAuth2 callback. Upsert semantics: first callback wins, subsequent callbacks replace tokens.';

COMMENT ON CONSTRAINT fk_user_sessions_service ON user_sessions IS
'Foreign key to thirdparty_oauth2_services with ON DELETE RESTRICT. Prevents deletion of services with active sessions (follows FR-022 pattern for referential integrity).';
