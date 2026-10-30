-- Migration: Drop user_grants table
-- Feature: 20251217-120000-domain-model-apis
-- Description: Drops the user_grants table and associated indexes

-- Drop indexes
DROP INDEX IF EXISTS idx_grants_tokens;
DROP INDEX IF EXISTS idx_grants_active;
DROP INDEX IF EXISTS idx_grants_agent;
DROP INDEX IF EXISTS idx_grants_principal;

-- Drop table
DROP TABLE IF EXISTS user_grants CASCADE;
