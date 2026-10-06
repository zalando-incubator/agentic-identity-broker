CREATE INDEX CONCURRENTLY idx_refresh_token_sessions_agent_request_id ON refresh_token_sessions (agent_id, request_id)
    WHERE used_at IS NULL;
