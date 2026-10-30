-- Migration: Drop agents table
-- Feature: 20251217-120000-domain-model-apis
-- Description: Drops the agents table and associated indexes

-- Drop indexes
DROP INDEX IF EXISTS idx_agents_external_id;
DROP INDEX IF EXISTS idx_agents_client_id;

-- Drop table
DROP TABLE IF EXISTS agents CASCADE;
