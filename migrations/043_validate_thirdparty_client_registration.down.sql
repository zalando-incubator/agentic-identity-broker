SET LOCAL statement_timeout = '1s';

DO $migration$
DECLARE
    definition TEXT;
BEGIN
    LOCK TABLE thirdparty_oauth2_services IN ACCESS EXCLUSIVE MODE NOWAIT;

    SELECT pg_get_constraintdef(oid) INTO definition
    FROM pg_constraint
    WHERE conrelid = 'thirdparty_oauth2_services'::regclass
        AND conname = 'chk_thirdparty_oauth2_services_client_registration';

    IF definition IS NULL THEN
        RAISE EXCEPTION 'client registration constraint is missing';
    END IF;

    EXECUTE format('ALTER TABLE thirdparty_oauth2_services DROP CONSTRAINT %I, ADD CONSTRAINT %I %s NOT VALID',
        'chk_thirdparty_oauth2_services_client_registration',
        'chk_thirdparty_oauth2_services_client_registration', definition);
END
$migration$;
