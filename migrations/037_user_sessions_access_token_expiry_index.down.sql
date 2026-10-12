-- migrate:no-transaction
DROP INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at;
