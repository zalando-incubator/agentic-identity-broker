CREATE TABLE domain_events (
    id          UUID        PRIMARY KEY,
    event       TEXT        NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    principal   TEXT,
    actor       TEXT,
    trace_id    TEXT,
    attributes  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_domain_events_event_not_empty CHECK (event <> ''),
    CONSTRAINT chk_domain_events_attributes_object CHECK (jsonb_typeof(attributes) = 'object')
);

CREATE INDEX idx_domain_events_recorded
    ON domain_events (recorded_at, id);

CREATE INDEX idx_domain_events_principal
    ON domain_events (principal, occurred_at DESC, id DESC);

CREATE INDEX idx_domain_events_agent
    ON domain_events ((attributes->>'agent_id'), occurred_at DESC, id DESC);
