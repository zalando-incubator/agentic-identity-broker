-- Migration: 004_create_user_sessions.down.sql
-- Purpose: Rollback user_sessions table creation
-- Feature: 20251222-120000-thirdparty-oauth2-sessions
-- Date: 2025-12-23

-- Drop trigger first (depends on function)
DROP TRIGGER IF EXISTS trigger_user_sessions_updated_at ON user_sessions;

-- Drop function
DROP FUNCTION IF EXISTS update_user_sessions_updated_at();

-- Drop table (cascades to indexes and constraints)
DROP TABLE IF EXISTS user_sessions;
