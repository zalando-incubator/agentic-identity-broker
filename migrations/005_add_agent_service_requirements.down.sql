-- Migration 005 Rollback: Remove service_requirements column from agents table
-- Feature: Agent Permission Requirements (Spec 20260108-120000)
-- WARNING: This rollback will DELETE all service requirement data

-- Remove GIN index first (must drop index before dropping column)
DROP INDEX IF EXISTS idx_agents_service_requirements;

-- Remove service_requirements column
-- WARNING: This will permanently delete all service requirement configurations
ALTER TABLE agents DROP COLUMN IF EXISTS service_requirements;
