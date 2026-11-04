SET LOCAL statement_timeout = '5s';

DO $migration$
BEGIN
    LOCK TABLE agents IN ACCESS EXCLUSIVE MODE NOWAIT;
    CREATE INDEX idx_agents_external_id ON agents (external_id) WHERE external_id IS NOT NULL;
    ALTER TABLE agents DROP CONSTRAINT chk_agents_external_id_not_empty;
    DROP INDEX uq_agents_external_id;
END;
$migration$;
