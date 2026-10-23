SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services IN ACCESS EXCLUSIVE MODE NOWAIT;

    IF EXISTS (
        SELECT 1 FROM thirdparty_oauth2_services
        WHERE client_registration = 'cimd' AND token_endpoint_auth_method = 'none'
    ) THEN
        RAISE EXCEPTION 'cannot rollback client registration while CIMD public services exist';
    END IF;

    ALTER TABLE thirdparty_oauth2_services
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_registration,
        DROP COLUMN client_registration;
END
$migration$;
