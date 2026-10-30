-- Migration 005: Add service_requirements column to agents table
-- Feature: Agent Permission Requirements (Spec 20260108-120000)
-- Purpose: Allow agents to declare mandatory/optional third-party service requirements
--
-- Schema Change:
-- - Adds service_requirements JSONB column (nullable for backward compatibility)
-- - Adds GIN index for efficient JSONB queries
--
-- Backward Compatibility:
-- - NULL value indicates no service requirements (default for existing agents)
-- - Existing agents continue to work without modification

-- Add service_requirements JSONB column to agents table
ALTER TABLE agents 
  ADD COLUMN service_requirements JSONB DEFAULT NULL;

-- Add GIN index for efficient JSONB queries on service_requirements
-- This enables fast queries like: WHERE service_requirements @> '[{"service_id": "..."}]'
CREATE INDEX idx_agents_service_requirements 
  ON agents USING GIN (service_requirements);

-- Add column comment explaining structure and backward compatibility
COMMENT ON COLUMN agents.service_requirements IS 
  'JSONB array of ServiceRequirement objects {service_id: UUID, requirement_type: "mandatory"|"optional", required_scopes: string[]}. NULL = no requirements (backward compatible).';
