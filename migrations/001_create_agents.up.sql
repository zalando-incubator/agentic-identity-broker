-- Migration: Create agents table
-- Feature: 20251217-120000-domain-model-apis
-- Description: Creates the agents table for storing AI agent configurations

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create agents table
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY,
    client_id VARCHAR(255) NOT NULL UNIQUE,
    external_id VARCHAR(255),
    display_name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    governance_url TEXT,
    user_documentation_url TEXT,
    agent_interface_url TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_agents_client_id ON agents(client_id);
CREATE INDEX IF NOT EXISTS idx_agents_external_id ON agents(external_id) WHERE external_id IS NOT NULL;

-- Add comment describing the table
COMMENT ON TABLE agents IS 'Stores AI agent configurations registered in the identity broker';
COMMENT ON COLUMN agents.id IS 'Unique identifier (UUID)';
COMMENT ON COLUMN agents.client_id IS 'OAuth2 client ID for this agent (unique)';
COMMENT ON COLUMN agents.external_id IS 'Optional external governance system identifier';
COMMENT ON COLUMN agents.display_name IS 'Human-readable display name';
COMMENT ON COLUMN agents.description IS 'Human-readable description of agent purpose';
COMMENT ON COLUMN agents.governance_url IS 'Optional URL to governance documentation';
COMMENT ON COLUMN agents.user_documentation_url IS 'Optional URL to user documentation';
COMMENT ON COLUMN agents.agent_interface_url IS 'Optional URL to agent interface/chat';
