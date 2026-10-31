-- Feature 055 (DEC-004): pin granted Virtual Service Scopes to service UUIDs.
ALTER TABLE authorization_codes ADD COLUMN service_scope_bindings JSONB NULL;
ALTER TABLE refresh_token_sessions ADD COLUMN service_scope_bindings JSONB NULL;
