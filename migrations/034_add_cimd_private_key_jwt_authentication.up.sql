ALTER TABLE thirdparty_oauth2_services
    DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
    ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
        CHECK (
            (token_endpoint_auth_method IS NULL AND client_secret_encrypted IS NOT NULL)
            OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
            OR (token_endpoint_auth_method = 'private_key_jwt' AND client_secret_encrypted IS NULL)
        );
