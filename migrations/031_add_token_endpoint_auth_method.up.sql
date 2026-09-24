ALTER TABLE thirdparty_oauth2_services
    ALTER COLUMN client_secret_encrypted DROP NOT NULL;

ALTER TABLE thirdparty_oauth2_services
    ADD COLUMN token_endpoint_auth_method VARCHAR(32);

ALTER TABLE thirdparty_oauth2_services
    ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
    CHECK (
        (token_endpoint_auth_method IS NULL AND client_secret_encrypted IS NOT NULL)
        OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
    );
