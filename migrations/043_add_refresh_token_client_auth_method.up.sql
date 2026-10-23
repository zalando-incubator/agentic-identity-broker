ALTER TABLE refresh_token_sessions
    ADD COLUMN client_auth_method TEXT NULL;

ALTER TABLE refresh_token_sessions
    ADD CONSTRAINT chk_refresh_token_sessions_client_auth_method
    CHECK (client_auth_method IS NULL OR client_auth_method = 'private_key_jwt') NOT VALID;
