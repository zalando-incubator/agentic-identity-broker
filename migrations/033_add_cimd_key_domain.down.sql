DO $$
DECLARE
    blocking_kids TEXT;
BEGIN
    SELECT string_agg(kid, ', ' ORDER BY kid)
    INTO blocking_kids
    FROM signing_keys
    WHERE key_domain = 'cimd_client_authentication';

    IF blocking_kids IS NOT NULL THEN
        RAISE EXCEPTION 'cannot revert CIMD key domain while CIMD client-authentication keys exist: %', blocking_kids;
    END IF;
END
$$;

DROP INDEX IF EXISTS idx_signing_keys_single_current_active_per_domain;

CREATE UNIQUE INDEX idx_signing_keys_single_current_active
    ON signing_keys (is_current)
    WHERE removed_at IS NULL AND is_current = true;

ALTER TABLE signing_keys
    DROP CONSTRAINT chk_signing_keys_key_domain,
    DROP COLUMN key_domain;
