ALTER TABLE signing_keys ADD COLUMN public_jwk BYTEA;

CREATE TABLE signing_key_set_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    version BIGINT NOT NULL DEFAULT 0
);

INSERT INTO signing_key_set_state (id, version) VALUES (1, 0);

CREATE FUNCTION bump_signing_key_set_version() RETURNS TRIGGER AS $$
BEGIN
    UPDATE signing_key_set_state SET version = version + 1 WHERE id = 1;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER signing_key_set_version_changed
AFTER INSERT OR UPDATE OR DELETE ON signing_keys
FOR EACH ROW EXECUTE FUNCTION bump_signing_key_set_version();
