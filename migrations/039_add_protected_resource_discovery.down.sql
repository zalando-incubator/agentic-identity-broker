LOCK TABLE thirdparty_oauth2_services IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM thirdparty_oauth2_services
        WHERE resource_url IS NOT NULL OR client_method IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'cannot revert protected-resource discovery while discovery-backed services exist';
    END IF;
END
$$;

DROP INDEX ux_thirdparty_oauth2_services_dcr_issuer_client_id;

ALTER TABLE thirdparty_oauth2_services
    DROP CONSTRAINT chk_thirdparty_oauth2_services_discovery_fields,
    DROP CONSTRAINT chk_thirdparty_oauth2_services_discovery_status,
    DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
    ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
        CHECK (
            (token_endpoint_auth_method IS NULL AND client_secret_encrypted IS NOT NULL)
            OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
            OR (token_endpoint_auth_method = 'private_key_jwt' AND client_secret_encrypted IS NULL)
        ),
    DROP COLUMN resource_url,
    DROP COLUMN client_method,
    DROP COLUMN resource_explicit,
    DROP COLUMN discovery_last_attempt_at,
    DROP COLUMN discovery_last_success_at,
    DROP COLUMN discovery_failure_reason;
