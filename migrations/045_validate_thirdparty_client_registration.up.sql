DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services IN SHARE UPDATE EXCLUSIVE MODE NOWAIT;

    UPDATE thirdparty_oauth2_services
    SET client_registration = 'cimd'
    WHERE token_endpoint_auth_method = 'private_key_jwt'
        AND client_registration IS DISTINCT FROM 'cimd';

    ALTER TABLE thirdparty_oauth2_services
        VALIDATE CONSTRAINT chk_thirdparty_oauth2_services_client_registration;
END
$migration$;
