-- RFC 6749 section 2.2 leaves the client identifier size undefined.
-- Stateless DCR servers can issue client IDs longer than 255 characters.
-- A long client ID can exceed the B-tree entry limit, so the DCR unique index
-- keys on a fixed-size digest. A collision can only reject a registration.
SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services IN ACCESS EXCLUSIVE MODE NOWAIT;

    DROP INDEX ux_thirdparty_oauth2_services_dcr_issuer_client_id;

    ALTER TABLE thirdparty_oauth2_services
        ALTER COLUMN client_id TYPE TEXT;

    CREATE UNIQUE INDEX ux_thirdparty_oauth2_services_dcr_issuer_client_id
        ON thirdparty_oauth2_services (issuer_uri, md5(client_id))
        WHERE client_method = 'dcr';
END
$migration$;
