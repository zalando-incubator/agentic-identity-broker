CREATE TABLE business_events (
    id          UUID        PRIMARY KEY,
    event       TEXT        NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    principal   TEXT,
    actor       TEXT,
    trace_id    TEXT,
    attributes  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_business_events_event_not_empty CHECK (event <> ''),
    CONSTRAINT chk_business_events_attributes_object CHECK (jsonb_typeof(attributes) = 'object')
);

CREATE INDEX idx_business_events_recorded
    ON business_events (recorded_at, id);

CREATE INDEX idx_business_events_principal
    ON business_events (principal, occurred_at DESC, id DESC);

CREATE INDEX idx_business_events_agent
    ON business_events ((attributes->>'agent_id'), occurred_at DESC, id DESC);
