ALTER TABLE refresh_token_sessions
    DROP CONSTRAINT chk_refresh_token_sessions_client_auth_method;

ALTER TABLE refresh_token_sessions
    DROP COLUMN client_auth_method;
