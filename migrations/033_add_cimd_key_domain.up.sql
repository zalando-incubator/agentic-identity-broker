ALTER TABLE signing_keys
    ADD COLUMN key_domain VARCHAR(32);

UPDATE signing_keys
SET key_domain = 'token_signing'
WHERE key_domain IS NULL;

ALTER TABLE signing_keys
    ALTER COLUMN key_domain SET NOT NULL,
    ADD CONSTRAINT chk_signing_keys_key_domain
        CHECK (key_domain IN ('token_signing', 'cimd_client_authentication'));

DROP INDEX IF EXISTS idx_signing_keys_single_current_active;

CREATE UNIQUE INDEX idx_signing_keys_single_current_active_per_domain
    ON signing_keys (key_domain)
    WHERE removed_at IS NULL AND is_current = true;
