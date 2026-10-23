-- Add the column and unvalidated guard without scanning existing services under this lock.
SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services IN ACCESS EXCLUSIVE MODE NOWAIT;

    ALTER TABLE thirdparty_oauth2_services
        ADD COLUMN client_registration TEXT NOT NULL DEFAULT 'static',
        ADD CONSTRAINT chk_thirdparty_oauth2_services_client_registration
            CHECK ((
                (
                    client_registration = 'static'
                    AND token_endpoint_auth_method IS DISTINCT FROM 'private_key_jwt'
                    AND client_method IS DISTINCT FROM 'cimd'
                )
                OR (
                    client_registration = 'cimd'
                    AND token_endpoint_auth_method IN ('private_key_jwt', 'none')
                    AND client_secret_encrypted IS NULL
                    AND credential_source = 'stored'
                    AND client_method IS DISTINCT FROM 'dcr'
                    AND client_id IS NOT NULL
                    AND right(client_id, char_length('/.well-known/oauth-client/' || id::text)) =
                        '/.well-known/oauth-client/' || id::text
                )
            ) IS TRUE) NOT VALID;
END
$migration$;
