-- Rolling back discards every Service-Scope Binding; roll back the code with the schema.
ALTER TABLE refresh_token_sessions DROP COLUMN service_scope_bindings;
ALTER TABLE authorization_codes DROP COLUMN service_scope_bindings;
