DO $migration$
BEGIN
    LOCK TABLE public.user_session_state IN ACCESS EXCLUSIVE MODE NOWAIT;
    LOCK TABLE public.user_sessions IN ACCESS EXCLUSIVE MODE NOWAIT;
    IF EXISTS (SELECT 1 FROM public.user_sessions WHERE refresh_token_rejected) THEN
        RAISE EXCEPTION 'cannot roll back session validity while rejected sessions remain';
    END IF;

    CREATE OR REPLACE VIEW public.user_session_state WITH (security_invoker = true) AS
        SELECT id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
               token_type, access_token_expires_at, FALSE AS refresh_token_rejected, scope,
               encryption_context, initiated_at, created_at, updated_at
        FROM public.user_sessions;

    CREATE OR REPLACE FUNCTION public.upsert_user_session_state(
        session_id UUID, principal TEXT, service_id UUID, encrypted_access_token BYTEA,
        encrypted_refresh_token BYTEA, token_type TEXT, access_token_expires_at TIMESTAMPTZ,
        refresh_token_rejected BOOLEAN, scope TEXT[], encryption_context JSONB,
        initiated_at TIMESTAMPTZ, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
    ) RETURNS VOID LANGUAGE plpgsql SECURITY INVOKER AS $function$
    BEGIN
        IF $8 THEN
            RAISE EXCEPTION 'session rejection persistence requires migration 039';
        END IF;
        INSERT INTO public.user_sessions (
            id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
            token_type, access_token_expires_at, scope, encryption_context,
            initiated_at, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $9, $10, $11, $12, $13)
        ON CONFLICT ON CONSTRAINT user_sessions_principal_service_unique DO UPDATE SET
            encrypted_access_token = EXCLUDED.encrypted_access_token,
            encrypted_refresh_token = EXCLUDED.encrypted_refresh_token,
            token_type = EXCLUDED.token_type,
            access_token_expires_at = EXCLUDED.access_token_expires_at,
            scope = EXCLUDED.scope,
            encryption_context = EXCLUDED.encryption_context,
            updated_at = NOW();
    END
    $function$;

    CREATE OR REPLACE FUNCTION public.update_user_session_state(
        session_id UUID, encrypted_access_token BYTEA, encrypted_refresh_token BYTEA,
        access_token_expires_at TIMESTAMPTZ, refresh_token_rejected BOOLEAN, updated_at TIMESTAMPTZ
    ) RETURNS VOID LANGUAGE plpgsql SECURITY INVOKER AS $function$
    BEGIN
        IF $5 THEN
            RAISE EXCEPTION 'session rejection persistence requires migration 039';
        END IF;
        UPDATE public.user_sessions SET
            encrypted_access_token = $2,
            encrypted_refresh_token = $3,
            access_token_expires_at = $4,
            updated_at = $6
        WHERE id = $1;
    END
    $function$;
    ALTER TABLE public.user_sessions DROP COLUMN refresh_token_rejected;
END
$migration$;
