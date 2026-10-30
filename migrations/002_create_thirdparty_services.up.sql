-- Migration: Create thirdparty_oauth2_services table
-- Feature: 20251217-120000-domain-model-apis
-- Description: Creates the table for storing third-party OAuth2 service configurations

CREATE TABLE IF NOT EXISTS thirdparty_oauth2_services (
    id UUID PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    client_id VARCHAR(255) NOT NULL,
    client_secret_encrypted BYTEA NOT NULL,  -- AES-256-GCM encrypted client secret
    issuer_uri TEXT NOT NULL,
    enable_discovery BOOLEAN NOT NULL DEFAULT true,
    metadata_url TEXT,
    token_endpoint TEXT,
    authorize_endpoint TEXT,
    scopes JSONB NOT NULL,  -- Array of {scope_value, description}
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_services_display_name ON thirdparty_oauth2_services(display_name);
CREATE INDEX IF NOT EXISTS idx_services_scopes ON thirdparty_oauth2_services USING GIN(scopes);

-- Add comments describing the table
COMMENT ON TABLE thirdparty_oauth2_services IS 'Stores third-party OAuth2 service configurations (GitHub, Google, etc.)';
COMMENT ON COLUMN thirdparty_oauth2_services.id IS 'Unique identifier (UUID)';
COMMENT ON COLUMN thirdparty_oauth2_services.display_name IS 'Human-readable service name';
COMMENT ON COLUMN thirdparty_oauth2_services.client_id IS 'OAuth2 client ID for this service';
COMMENT ON COLUMN thirdparty_oauth2_services.client_secret_encrypted IS 'AES-256-GCM encrypted client secret';
COMMENT ON COLUMN thirdparty_oauth2_services.issuer_uri IS 'OAuth2 issuer URI (must be HTTPS)';
COMMENT ON COLUMN thirdparty_oauth2_services.enable_discovery IS 'Enable automatic OAuth2 endpoint discovery';
COMMENT ON COLUMN thirdparty_oauth2_services.metadata_url IS 'Optional custom OAuth2 metadata URL';
COMMENT ON COLUMN thirdparty_oauth2_services.token_endpoint IS 'OAuth2 token endpoint';
COMMENT ON COLUMN thirdparty_oauth2_services.authorize_endpoint IS 'OAuth2 authorization endpoint';
COMMENT ON COLUMN thirdparty_oauth2_services.scopes IS 'JSONB array of available OAuth2 scopes with descriptions';
