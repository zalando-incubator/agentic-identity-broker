DO $migration$
BEGIN
    LOCK TABLE public.user_session_state IN ACCESS EXCLUSIVE MODE NOWAIT;
    LOCK TABLE public.user_sessions IN ACCESS EXCLUSIVE MODE NOWAIT;
    ALTER TABLE public.user_sessions DROP COLUMN refresh_token_expires_at;
END
$migration$;
