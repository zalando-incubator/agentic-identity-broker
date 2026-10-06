CREATE INDEX CONCURRENTLY idx_refresh_token_sessions_rootless_expiry ON refresh_token_sessions (expires_at, signature)
    WHERE session_id IS NULL;
