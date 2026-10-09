SET lock_timeout = '5s';
ALTER TABLE refresh_token_sessions ADD COLUMN grant_id UUID;
