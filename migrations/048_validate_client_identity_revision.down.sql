SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services, user_sessions
        IN ACCESS EXCLUSIVE MODE NOWAIT;

    ALTER TABLE thirdparty_oauth2_services
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_identity,
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_identity_revision,
        ADD CONSTRAINT chk_thirdparty_oauth2_services_client_identity
            CHECK (client_identity IS NULL OR client_identity <> '') NOT VALID,
        ADD CONSTRAINT chk_thirdparty_oauth2_services_client_identity_revision
            CHECK (client_identity_revision >= 0) NOT VALID;

    ALTER TABLE user_sessions
        DROP CONSTRAINT chk_user_sessions_upstream_client_revision,
        ADD CONSTRAINT chk_user_sessions_upstream_client_revision
            CHECK (upstream_client_revision >= 0) NOT VALID;
END
$migration$;
