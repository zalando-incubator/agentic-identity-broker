CREATE TABLE client_assertion_replays (
    client_id  TEXT        NOT NULL,
    jti        TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (client_id, jti)
);

CREATE INDEX idx_client_assertion_replays_expires_at
    ON client_assertion_replays (expires_at);
