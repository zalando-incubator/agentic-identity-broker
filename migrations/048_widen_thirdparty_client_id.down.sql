-- Fails while any stored client ID is longer than 255 characters.
SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services IN ACCESS EXCLUSIVE MODE NOWAIT;

    ALTER TABLE thirdparty_oauth2_services
        ALTER COLUMN client_id TYPE VARCHAR(255);
END
$migration$;
