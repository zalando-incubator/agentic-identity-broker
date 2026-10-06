CREATE INDEX CONCURRENTLY idx_refresh_token_sessions_session_id ON refresh_token_sessions (session_id)
    WHERE session_id IS NOT NULL;
