-- migrate:no-transaction
CREATE INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at ON user_sessions (access_token_expires_at, id);
