SET LOCAL statement_timeout = '5s';

DO $migration$
DECLARE
    duplicate_groups jsonb;
    duplicate_count bigint;
    empty_agents jsonb;
    empty_count bigint;
BEGIN
    LOCK TABLE agents IN ACCESS EXCLUSIVE MODE NOWAIT;

    WITH duplicates AS (
        SELECT external_id, array_agg(id ORDER BY created_at, id) AS agent_ids
        FROM agents
        WHERE external_id IS NOT NULL AND external_id <> ''
        GROUP BY external_id
        HAVING COUNT(*) > 1
    ), first_groups AS (
        SELECT external_id, agent_ids FROM duplicates ORDER BY external_id LIMIT 100
    )
    SELECT (SELECT COUNT(*) FROM duplicates),
           (SELECT jsonb_agg(jsonb_build_object('external_id', external_id, 'agent_ids', agent_ids) ORDER BY external_id) FROM first_groups)
    INTO duplicate_count, duplicate_groups;

    SELECT COUNT(*) INTO empty_count FROM agents WHERE external_id = '';
    SELECT jsonb_agg(id ORDER BY created_at, id) INTO empty_agents
    FROM (SELECT id, created_at FROM agents WHERE external_id = '' ORDER BY created_at, id LIMIT 100) AS first_empty;

    IF duplicate_count > 0 OR empty_count > 0 THEN
        RAISE EXCEPTION 'agent external_id uniqueness migration blocked'
            USING DETAIL = format('duplicate groups: %s; remaining duplicate groups: %s; empty-value agent IDs: %s; remaining empty-value agents: %s',
                COALESCE(duplicate_groups, '[]'::jsonb), GREATEST(duplicate_count - 100, 0),
                COALESCE(empty_agents, '[]'::jsonb), GREATEST(empty_count - 100, 0)),
            HINT = 'See docs/operations/agent-external-id-uniqueness.md; no agents were changed';
    END IF;

    CREATE UNIQUE INDEX uq_agents_external_id ON agents (external_id) WHERE external_id IS NOT NULL;
    ALTER TABLE agents ADD CONSTRAINT chk_agents_external_id_not_empty CHECK (external_id <> '');
    DROP INDEX idx_agents_external_id;
END;
$migration$;
