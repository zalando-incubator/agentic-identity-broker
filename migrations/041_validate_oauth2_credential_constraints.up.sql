DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services, user_sessions
        IN SHARE UPDATE EXCLUSIVE MODE NOWAIT;

    ALTER TABLE thirdparty_oauth2_services
        VALIDATE CONSTRAINT chk_thirdparty_oauth2_services_credential_source,
        VALIDATE CONSTRAINT chk_thirdparty_oauth2_services_client_auth;

    ALTER TABLE user_sessions
        VALIDATE CONSTRAINT chk_user_sessions_upstream_client_id;
END
$migration$;
