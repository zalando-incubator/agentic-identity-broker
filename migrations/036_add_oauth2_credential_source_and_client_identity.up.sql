DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services, user_sessions
        IN ACCESS EXCLUSIVE MODE;

    ALTER TABLE thirdparty_oauth2_services
        ADD COLUMN credential_source TEXT NOT NULL DEFAULT 'stored',
        ADD COLUMN credential_source_transitioned BOOLEAN NOT NULL DEFAULT FALSE,
        ALTER COLUMN client_id DROP NOT NULL,
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
        ADD CONSTRAINT chk_thirdparty_oauth2_services_credential_source
            CHECK (credential_source IN ('stored', 'filesystem')),
        ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
            CHECK ((
                (
                    credential_source = 'stored'
                    AND client_id IS NOT NULL
                    AND client_id <> ''
                    AND (
                        (
                            token_endpoint_auth_method IS NULL
                            AND client_secret_encrypted IS NOT NULL
                            AND octet_length(client_secret_encrypted) > 0
                        )
                        OR (
                            token_endpoint_auth_method IS NOT DISTINCT FROM 'none'
                            AND client_secret_encrypted IS NULL
                        )
                        OR (
                            token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt'
                            AND client_secret_encrypted IS NULL
                        )
                    )
                )
                OR (
                    credential_source = 'filesystem'
                    AND canonical_id IS NOT NULL
                    AND canonical_id COLLATE "C" ~ '^[A-Za-z0-9._-]{1,128}$'
                    AND (
                        CASE WHEN char_length(canonical_id) = 38
                            THEN substring(canonical_id FROM 2 FOR 36)
                            ELSE canonical_id
                        END
                    ) COLLATE "C" !~ '^([0-9A-Fa-f]{32}|[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12})$'
                    AND client_id IS NULL
                    AND client_secret_encrypted IS NULL
                    AND token_endpoint_auth_method IS NULL
                    AND oauth2_flavor IN ('', 'standard', 'github')
                )
            ) IS TRUE);

    ALTER TABLE user_sessions
        ADD COLUMN upstream_client_id TEXT,
        ADD CONSTRAINT chk_user_sessions_upstream_client_id
            CHECK (upstream_client_id IS NULL OR upstream_client_id <> '');
END
$migration$;
