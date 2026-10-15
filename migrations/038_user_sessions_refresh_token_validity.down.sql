DO $migration$
BEGIN
    LOCK TABLE public.user_session_state IN ACCESS EXCLUSIVE MODE NOWAIT;
    LOCK TABLE public.user_sessions IN ACCESS EXCLUSIVE MODE NOWAIT;
    DROP FUNCTION public.update_user_session_state(UUID, BYTEA, BYTEA, TIMESTAMPTZ, BOOLEAN, TIMESTAMPTZ);
    DROP FUNCTION public.upsert_user_session_state(UUID, TEXT, UUID, BYTEA, BYTEA, TEXT, TIMESTAMPTZ, BOOLEAN, TEXT[], JSONB, TIMESTAMPTZ, TIMESTAMPTZ, TIMESTAMPTZ);
    DROP VIEW public.user_session_state;
END
$migration$;
