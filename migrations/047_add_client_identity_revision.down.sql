SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services, user_sessions
        IN ACCESS EXCLUSIVE MODE NOWAIT;

    IF EXISTS (SELECT 1 FROM thirdparty_oauth2_services WHERE client_identity_revision > 0) THEN
        RAISE EXCEPTION 'Cannot remove client identity revisions after an identity change';
    END IF;

    ALTER TABLE thirdparty_oauth2_services
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_identity,
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_identity_revision,
        DROP COLUMN client_identity,
        DROP COLUMN client_identity_revision;

    ALTER TABLE user_sessions
        DROP CONSTRAINT chk_user_sessions_upstream_client_revision,
        DROP COLUMN upstream_client_revision;
END
$migration$;
