DROP TRIGGER signing_key_set_version_changed ON signing_keys;
DROP FUNCTION bump_signing_key_set_version();
DROP TABLE signing_key_set_state;

ALTER TABLE signing_keys DROP COLUMN public_jwk;
