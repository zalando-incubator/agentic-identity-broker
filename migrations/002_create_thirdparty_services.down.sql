-- Migration: Drop thirdparty_oauth2_services table
-- Feature: 20251217-120000-domain-model-apis
-- Description: Drops the thirdparty_oauth2_services table and associated indexes

-- Drop indexes
DROP INDEX IF EXISTS idx_services_scopes;
DROP INDEX IF EXISTS idx_services_display_name;

-- Drop table
DROP TABLE IF EXISTS thirdparty_oauth2_services CASCADE;
