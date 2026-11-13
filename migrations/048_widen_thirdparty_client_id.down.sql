-- Fails while any stored client ID is longer than 255 characters.
SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services IN ACCESS EXCLUSIVE MODE NOWAIT;

    DROP INDEX ux_thirdparty_oauth2_services_dcr_issuer_client_id;

    ALTER TABLE thirdparty_oauth2_services
        ALTER COLUMN client_id TYPE VARCHAR(255);

    CREATE UNIQUE INDEX ux_thirdparty_oauth2_services_dcr_issuer_client_id
        ON thirdparty_oauth2_services (issuer_uri, client_id)
        WHERE client_method = 'dcr';
END
$migration$;
