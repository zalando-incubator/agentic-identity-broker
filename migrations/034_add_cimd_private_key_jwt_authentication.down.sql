DO $$
DECLARE
    blocking_services TEXT;
BEGIN
    SELECT string_agg(display_name, ', ' ORDER BY display_name)
    INTO blocking_services
    FROM thirdparty_oauth2_services
    WHERE token_endpoint_auth_method = 'private_key_jwt';

    IF blocking_services IS NOT NULL THEN
        RAISE EXCEPTION 'cannot revert CIMD private_key_jwt authentication while CIMD services exist: %', blocking_services;
    END IF;
END
$$;

ALTER TABLE thirdparty_oauth2_services
    DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
    ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
        CHECK (
            (token_endpoint_auth_method IS NULL AND client_secret_encrypted IS NOT NULL)
            OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
        );
