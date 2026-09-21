DO $$
DECLARE
    blocking_services TEXT;
BEGIN
    SELECT string_agg(display_name, ', ' ORDER BY display_name)
    INTO blocking_services
    FROM thirdparty_oauth2_services
    WHERE token_endpoint_auth_method IS NOT NULL;

    IF blocking_services IS NOT NULL THEN
        RAISE EXCEPTION 'cannot revert public client authentication while public services exist: %', blocking_services;
    END IF;
END
$$;

ALTER TABLE thirdparty_oauth2_services
    DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
    DROP COLUMN token_endpoint_auth_method,
    ALTER COLUMN client_secret_encrypted SET NOT NULL;
