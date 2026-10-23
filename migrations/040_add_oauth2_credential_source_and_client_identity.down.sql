SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services, user_sessions
        IN ACCESS EXCLUSIVE MODE NOWAIT;

    IF EXISTS (
        SELECT 1
        FROM thirdparty_oauth2_services
        WHERE credential_source IS DISTINCT FROM 'stored'
            OR credential_source_transitioned IS DISTINCT FROM FALSE
            OR client_id IS NULL
            OR client_id = ''
            OR (CASE
                WHEN client_method IS NULL THEN
                    (token_endpoint_auth_method IS NULL
                     AND client_secret_encrypted IS NOT NULL
                     AND octet_length(client_secret_encrypted) > 0)
                    OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none'
                        AND client_secret_encrypted IS NULL)
                    OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt'
                        AND client_secret_encrypted IS NULL)
                WHEN client_method = 'cimd' THEN
                    token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt'
                    AND client_secret_encrypted IS NULL
                WHEN client_method = 'dcr' THEN
                    (token_endpoint_auth_method IS NOT DISTINCT FROM 'none'
                     AND client_secret_encrypted IS NULL)
                    OR (token_endpoint_auth_method IS NOT NULL
                        AND token_endpoint_auth_method IN ('client_secret_basic', 'client_secret_post')
                        AND client_secret_encrypted IS NOT NULL
                        AND octet_length(client_secret_encrypted) > 0)
                ELSE FALSE
            END) IS NOT TRUE
    ) THEN
        RAISE EXCEPTION 'cannot revert OAuth2 credential sources while incompatible services or source-transition evidence exist';
    END IF;

    ALTER TABLE thirdparty_oauth2_services
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
        DROP CONSTRAINT chk_thirdparty_oauth2_services_credential_source,
        ALTER COLUMN client_id SET NOT NULL,
        ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
            CHECK (
                CASE
                    WHEN client_method IS NULL THEN
                        (token_endpoint_auth_method IS NULL AND client_secret_encrypted IS NOT NULL)
                        OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
                        OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt' AND client_secret_encrypted IS NULL)
                    WHEN client_method = 'cimd' THEN
                        token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt'
                        AND client_secret_encrypted IS NULL
                    WHEN client_method = 'dcr' THEN
                        (token_endpoint_auth_method IS NOT DISTINCT FROM 'none'
                         AND client_secret_encrypted IS NULL)
                        OR (token_endpoint_auth_method IS NOT NULL
                            AND token_endpoint_auth_method IN ('client_secret_basic', 'client_secret_post')
                            AND client_secret_encrypted IS NOT NULL)
                    ELSE FALSE
                END
            ),
        DROP COLUMN credential_source,
        DROP COLUMN credential_source_transitioned;

    -- Intentional data loss: established upstream client identities are dropped.
    -- Reapplying 040 does not restore them. Affected sessions retain NULL
    -- identities until users reconnect.
    ALTER TABLE user_sessions
        DROP CONSTRAINT chk_user_sessions_upstream_client_id,
        DROP COLUMN upstream_client_id;
END
$migration$;
