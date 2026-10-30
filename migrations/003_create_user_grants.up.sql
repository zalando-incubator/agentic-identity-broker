-- Migration: Create user_grants table
-- Feature: 20251217-120000-domain-model-apis
-- Description: Creates the table for storing user grants (permission delegations to agents)

CREATE TABLE IF NOT EXISTS user_grants (
    id UUID PRIMARY KEY,
    principal VARCHAR(255) NOT NULL,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    valid_until TIMESTAMP,  -- NULL = indefinite grant
    delegated_oauth2_tokens JSONB NOT NULL,  -- Array of {thirdparty_oauth2_service_id, scopes[]}
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- One grant per user-agent pair (upsert semantics)
    CONSTRAINT uq_principal_agent UNIQUE(principal, agent_id)
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_grants_principal ON user_grants(principal);
CREATE INDEX IF NOT EXISTS idx_grants_agent ON user_grants(agent_id);
-- Partial index on active grants (without NOW() to avoid IMMUTABLE requirement)
-- Application logic should filter expired grants at query time
CREATE INDEX IF NOT EXISTS idx_grants_active ON user_grants(principal, agent_id)
    WHERE valid_until IS NULL;
CREATE INDEX IF NOT EXISTS idx_grants_tokens ON user_grants USING GIN(delegated_oauth2_tokens);

-- Add comments describing the table
COMMENT ON TABLE user_grants IS 'Stores user grants (permission delegations to agents for third-party services)';
COMMENT ON COLUMN user_grants.id IS 'Unique identifier (UUID)';
COMMENT ON COLUMN user_grants.principal IS 'User identifier (from session)';
COMMENT ON COLUMN user_grants.agent_id IS 'Agent receiving delegation (foreign key with CASCADE DELETE)';
COMMENT ON COLUMN user_grants.valid_until IS 'Grant expiration timestamp (NULL = indefinite)';
COMMENT ON COLUMN user_grants.delegated_oauth2_tokens IS 'JSONB array of delegated third-party services and scopes';
COMMENT ON CONSTRAINT uq_principal_agent ON user_grants IS 'Enforces one grant per user-agent pair';
