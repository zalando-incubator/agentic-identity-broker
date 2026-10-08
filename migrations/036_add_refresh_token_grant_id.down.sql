SET lock_timeout = '5s';
ALTER TABLE refresh_token_sessions DROP COLUMN grant_id;
