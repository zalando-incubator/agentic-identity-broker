CREATE TABLE refresh_maintenance (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    database_id UUID NOT NULL DEFAULT gen_random_uuid(),
    lease_owner UUID,
    lease_until TIMESTAMPTZ NOT NULL DEFAULT '-infinity'
);

INSERT INTO refresh_maintenance (singleton) VALUES (TRUE);
