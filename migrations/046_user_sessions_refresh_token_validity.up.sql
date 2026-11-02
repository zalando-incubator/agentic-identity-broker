DO $migration$
BEGIN
    LOCK TABLE public.user_sessions IN ACCESS SHARE MODE NOWAIT;

    CREATE VIEW public.user_session_state WITH (security_invoker = true) AS
        SELECT id, principal, service_id, callback_id, upstream_client_id,
               encrypted_access_token, encrypted_refresh_token,
               token_type, access_token_expires_at, FALSE AS refresh_token_rejected, scope,
               encryption_context, initiated_at, created_at, updated_at
        FROM public.user_sessions;

    CREATE FUNCTION public.upsert_user_session_state(
        session_id UUID, principal TEXT, service_id UUID, encrypted_access_token BYTEA,
        encrypted_refresh_token BYTEA, token_type TEXT, access_token_expires_at TIMESTAMPTZ,
        refresh_token_rejected BOOLEAN, scope TEXT[], encryption_context JSONB,
        initiated_at TIMESTAMPTZ, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ,
        callback_id TEXT, upstream_client_id TEXT, expected_issuer_uri TEXT,
        expected_resource TEXT, replace_existing BOOLEAN
    ) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY INVOKER AS $function$
    BEGIN
        IF $8 THEN
            RAISE EXCEPTION 'session rejection persistence requires migration 047';
        END IF;
        INSERT INTO public.user_sessions (
            id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
            token_type, access_token_expires_at, scope, encryption_context,
            initiated_at, created_at, updated_at, callback_id, upstream_client_id
        )
        SELECT $1, $2, $3, $4, $5, $6, $7, $9, $10, $11, $12, $13, $14, $15
        FROM public.thirdparty_oauth2_services AS service
        WHERE service.id = $3
            AND ((service.resource_url IS NULL AND $16 = '' AND $17 = '')
                OR (service.resource_url IS NOT NULL AND $16 <> '' AND $17 <> ''
                    AND service.issuer_uri = $16 AND service.authorization_params ->> 'resource' = $17))
        FOR SHARE OF service
        ON CONFLICT ON CONSTRAINT user_sessions_principal_service_unique DO UPDATE SET
            encrypted_access_token = EXCLUDED.encrypted_access_token,
            encrypted_refresh_token = EXCLUDED.encrypted_refresh_token,
            refresh_token_expires_at = NULL,
            token_type = EXCLUDED.token_type,
            access_token_expires_at = EXCLUDED.access_token_expires_at,
            scope = EXCLUDED.scope,
            encryption_context = EXCLUDED.encryption_context,
            callback_id = EXCLUDED.callback_id,
            upstream_client_id = EXCLUDED.upstream_client_id,
            updated_at = NOW()
        WHERE $18;
        RETURN FOUND;
    END
    $function$;

    CREATE FUNCTION public.update_user_session_state(
        session_id UUID, encrypted_access_token BYTEA, encrypted_refresh_token BYTEA,
        access_token_expires_at TIMESTAMPTZ, refresh_token_rejected BOOLEAN, updated_at TIMESTAMPTZ,
        token_type TEXT, scope TEXT[], encryption_context JSONB, callback_id TEXT, upstream_client_id TEXT
    ) RETURNS VOID LANGUAGE plpgsql SECURITY INVOKER AS $function$
    BEGIN
        IF $5 THEN
            RAISE EXCEPTION 'session rejection persistence requires migration 047';
        END IF;
        UPDATE public.user_sessions SET
            encrypted_access_token = $2,
            encrypted_refresh_token = $3,
            refresh_token_expires_at = CASE
                WHEN user_sessions.encrypted_refresh_token IS DISTINCT FROM $3 THEN NULL
                ELSE user_sessions.refresh_token_expires_at
            END,
            access_token_expires_at = $4,
            updated_at = $6,
            token_type = $7,
            scope = $8,
            encryption_context = $9,
            callback_id = $10,
            upstream_client_id = $11
        WHERE id = $1;
    END
    $function$;
END
$migration$;
