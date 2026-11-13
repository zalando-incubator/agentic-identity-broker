-- RFC 6749 section 2.2 leaves the client identifier size undefined.
-- Stateless DCR servers can issue client IDs longer than 255 characters.
SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services IN ACCESS EXCLUSIVE MODE NOWAIT;

    ALTER TABLE thirdparty_oauth2_services
        ALTER COLUMN client_id TYPE TEXT;
END
$migration$;
