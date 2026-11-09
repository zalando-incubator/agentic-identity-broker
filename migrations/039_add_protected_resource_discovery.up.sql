ALTER TABLE thirdparty_oauth2_services
    ADD COLUMN resource_url TEXT,
    ADD COLUMN client_method VARCHAR(4),
    ADD COLUMN resource_explicit BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN discovery_last_attempt_at TIMESTAMPTZ,
    ADD COLUMN discovery_last_success_at TIMESTAMPTZ,
    ADD COLUMN discovery_failure_reason VARCHAR(64);

ALTER TABLE thirdparty_oauth2_services
    ADD CONSTRAINT chk_thirdparty_oauth2_services_discovery_fields CHECK (
        (
            resource_url IS NULL
            AND client_method IS NULL
            AND resource_explicit = FALSE
        ) OR (
            resource_url IS NOT NULL
            AND btrim(resource_url) <> ''
            AND enable_discovery IS TRUE
            AND metadata_url IS NULL
            AND client_method IS NOT NULL
            AND client_method IN ('cimd', 'dcr')
            AND authorization_params ? 'resource'
            AND jsonb_typeof(authorization_params -> 'resource') = 'string'
            AND NULLIF(btrim(authorization_params ->> 'resource'), '') IS NOT NULL
            AND (resource_explicit OR authorization_params ->> 'resource' = resource_url)
        )
    ),
    ADD CONSTRAINT chk_thirdparty_oauth2_services_discovery_status CHECK (
        (
            resource_url IS NULL
            AND discovery_last_attempt_at IS NULL
            AND discovery_last_success_at IS NULL
            AND discovery_failure_reason IS NULL
        ) OR (
            resource_url IS NOT NULL
            AND discovery_last_attempt_at IS NOT NULL
            AND discovery_last_success_at IS NOT NULL
            AND (
                (discovery_failure_reason IS NULL
                 AND discovery_last_attempt_at = discovery_last_success_at)
                OR (discovery_failure_reason IS NOT NULL
                    AND discovery_failure_reason ~ '^[a-z][a-z0-9_]*$'
                    AND discovery_last_attempt_at > discovery_last_success_at)
            )
        )
    );

ALTER TABLE thirdparty_oauth2_services
    DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
    ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth CHECK (
        CASE
            WHEN client_method IS NULL THEN
                (token_endpoint_auth_method IS NULL AND client_secret_encrypted IS NOT NULL)
                OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
                OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt' AND client_secret_encrypted IS NULL)
            WHEN client_method = 'cimd' THEN
                token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt'
                AND client_secret_encrypted IS NULL
            WHEN client_method = 'dcr' THEN
                (token_endpoint_auth_method IS NOT DISTINCT FROM 'none'
                 AND client_secret_encrypted IS NULL)
                OR (token_endpoint_auth_method IS NOT NULL
                    AND token_endpoint_auth_method IN ('client_secret_basic', 'client_secret_post')
                    AND client_secret_encrypted IS NOT NULL)
            ELSE FALSE
        END
    );

CREATE UNIQUE INDEX ux_thirdparty_oauth2_services_dcr_issuer_client_id
    ON thirdparty_oauth2_services (issuer_uri, client_id)
    WHERE client_method = 'dcr';
