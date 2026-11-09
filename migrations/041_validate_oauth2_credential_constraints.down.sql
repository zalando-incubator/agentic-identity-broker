SET LOCAL statement_timeout = '1s';

DO $migration$
DECLARE
    constraint_record RECORD;
BEGIN
    LOCK TABLE thirdparty_oauth2_services, user_sessions
        IN ACCESS EXCLUSIVE MODE NOWAIT;

    -- Restore 040's enforcement on new writes without scanning existing rows.
    FOR constraint_record IN
        SELECT conrelid::regclass AS relation, conname, pg_get_constraintdef(oid) AS definition
        FROM pg_constraint
        WHERE convalidated AND (
            (conrelid = 'thirdparty_oauth2_services'::regclass AND conname IN (
                'chk_thirdparty_oauth2_services_credential_source',
                'chk_thirdparty_oauth2_services_client_auth'))
            OR (conrelid = 'user_sessions'::regclass AND conname = 'chk_user_sessions_upstream_client_id')
        )
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I, ADD CONSTRAINT %I %s NOT VALID',
            constraint_record.relation, constraint_record.conname,
            constraint_record.conname, constraint_record.definition);
    END LOOP;
END
$migration$;
