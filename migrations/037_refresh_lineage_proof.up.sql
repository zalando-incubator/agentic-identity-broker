-- A per-root trust bit is a materialized proof, not a cache that can be refreshed
-- from the current token. Existing roots are checked over their entire ancestry
-- once; thereafter deferred row triggers invalidate it on any unsupported write.
ALTER TABLE refresh_sessions ADD COLUMN lineage_valid BOOLEAN NOT NULL DEFAULT TRUE;

WITH RECURSIVE ancestry AS (
    SELECT r.id, m.signature, m.predecessor_signature
    FROM refresh_sessions r
    JOIN refresh_token_sessions m ON m.signature = r.current_signature AND m.session_id = r.id
    UNION
    SELECT a.id, parent.signature, parent.predecessor_signature
    FROM ancestry a
    JOIN refresh_token_sessions parent ON parent.signature = a.predecessor_signature AND parent.session_id = a.id
), evidence AS (
    SELECT r.id, COUNT(DISTINCT a.signature) AS linked,
        BOOL_AND(
            child.signature IS NOT NULL AND mirror.signature IS NOT NULL
            AND mirror.request_id = r.id::text AND mirror.agent_id = r.agent_id
            AND mirror.principal = r.principal AND mirror.client_id = r.client_id
            AND mirror.scope = r.scope AND mirror.email IS NOT DISTINCT FROM r.email
            AND mirror.display_name = r.display_name
            AND mirror.created_at = child.issued_at AND mirror.expires_at = child.expires_at
            AND child.expires_at <= r.retain_until
            AND mirror.used_at IS NOT DISTINCT FROM child.used_at
            AND (a.signature <> r.original_token_signature OR
                (a.predecessor_signature IS NULL AND child.issued_at = r.started_at))
            AND (a.signature <> r.current_signature OR
                (child.used_at IS NULL AND child.issued_at = r.last_fresh_at
                    AND a.predecessor_signature IS NOT DISTINCT FROM r.previous_signature))
            AND (a.signature = r.current_signature OR child.used_at IS NOT NULL)
            AND (r.previous_signature IS NULL OR a.signature <> r.previous_signature
                OR child.used_at = r.previous_consumed_at)
            AND (successor.signature IS NULL OR successor.created_at = child.used_at)
        ) AS links_valid,
        BOOL_OR(a.signature = r.original_token_signature AND a.predecessor_signature IS NULL) AS anchored
    FROM refresh_sessions r
    LEFT JOIN ancestry a ON a.id = r.id
    LEFT JOIN refresh_tokens child ON child.signature = a.signature AND child.session_id = r.id
    LEFT JOIN refresh_token_sessions mirror ON mirror.signature = a.signature AND mirror.session_id = r.id
    LEFT JOIN refresh_token_sessions successor ON successor.session_id = r.id
        AND successor.predecessor_signature = a.signature
    GROUP BY r.id
)
UPDATE refresh_sessions r SET lineage_valid =
    e.links_valid IS TRUE AND e.anchored IS TRUE
    AND e.linked = (SELECT COUNT(*) FROM refresh_tokens WHERE session_id = r.id)
    AND e.linked = (SELECT COUNT(*) FROM refresh_token_sessions WHERE session_id = r.id)
    AND NOT EXISTS (SELECT 1 FROM refresh_token_sessions old
        WHERE old.agent_id = r.agent_id AND old.request_id = r.id::text AND old.session_id IS NULL)
    AND (r.terminal_reason IS NOT NULL OR EXISTS (SELECT 1 FROM user_grants grant_origin
        WHERE grant_origin.id = r.original_grant_id AND grant_origin.agent_id = r.agent_id
            AND grant_origin.principal = r.principal
            AND (grant_origin.valid_until IS NULL OR grant_origin.valid_until > r.started_at)))
FROM evidence e WHERE e.id = r.id;

-- Both sides of a new edge are written before the root moves its current pointer.
-- The trigger sees their final committed shape rather than transient insert order.
CREATE FUNCTION refresh_lineage_current_edge_valid(session_uuid UUID) RETURNS BOOLEAN
LANGUAGE sql AS $$
    SELECT EXISTS (
        SELECT 1 FROM refresh_sessions r
        JOIN refresh_tokens child ON child.signature = r.current_signature AND child.session_id = r.id
        JOIN refresh_token_sessions mirror ON mirror.signature = child.signature AND mirror.session_id = r.id
        WHERE r.id = session_uuid AND child.used_at IS NULL AND child.issued_at = r.last_fresh_at
            AND child.expires_at <= r.retain_until
            AND mirror.request_id = r.id::text AND mirror.agent_id = r.agent_id
            AND mirror.principal = r.principal AND mirror.client_id = r.client_id
            AND mirror.scope = r.scope AND mirror.email IS NOT DISTINCT FROM r.email
            AND mirror.display_name = r.display_name
            AND mirror.created_at = child.issued_at AND mirror.expires_at = child.expires_at
            AND mirror.used_at IS NULL AND mirror.predecessor_signature IS NOT DISTINCT FROM r.previous_signature
            AND (
                (r.previous_signature IS NULL AND child.signature = r.original_token_signature
                    AND child.issued_at = r.started_at)
                OR (r.previous_signature IS NOT NULL AND EXISTS (
                    SELECT 1 FROM refresh_tokens prior
                    JOIN refresh_token_sessions prior_mirror ON prior_mirror.signature = prior.signature
                        AND prior_mirror.session_id = r.id
                    WHERE prior.signature = r.previous_signature AND prior.session_id = r.id
                        AND prior.used_at = child.issued_at AND r.previous_consumed_at = child.issued_at
                        AND prior_mirror.used_at = prior.used_at
                        AND prior_mirror.request_id = r.id::text
                        AND prior_mirror.agent_id = r.agent_id AND prior_mirror.principal = r.principal
                        AND prior_mirror.client_id = r.client_id AND prior_mirror.scope = r.scope
                        AND prior_mirror.created_at = prior.issued_at
                        AND prior_mirror.expires_at = prior.expires_at
                ))
            )
    );
$$;

CREATE FUNCTION refresh_lineage_watch_child() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    session_uuid UUID;
    signature_value TEXT;
    root_row refresh_sessions%ROWTYPE;
    safe_change BOOLEAN := FALSE;
BEGIN
    IF TG_TABLE_NAME = 'refresh_tokens' THEN
        session_uuid := CASE WHEN TG_OP = 'INSERT' THEN NEW.session_id ELSE OLD.session_id END;
    ELSE
        session_uuid := CASE WHEN TG_OP = 'INSERT' THEN NEW.session_id ELSE OLD.session_id END;
        IF session_uuid IS NULL AND TG_OP IN ('INSERT', 'UPDATE') AND
            NEW.request_id ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN
            -- An old-only descendant of an anchored request has no new-issuer evidence.
            UPDATE refresh_sessions SET lineage_valid = FALSE
            WHERE id = NEW.request_id::uuid AND agent_id = NEW.agent_id AND terminal_reason IS NULL;
        END IF;
    END IF;
    -- Moving an existing row into another family (including NULL -> anchored)
    -- must poison the destination proof as well as its original owner.
    IF TG_OP = 'UPDATE' AND NEW.session_id IS DISTINCT FROM OLD.session_id AND NEW.session_id IS NOT NULL THEN
        UPDATE refresh_sessions SET lineage_valid = FALSE
        WHERE id = NEW.session_id AND terminal_reason IS NULL;
    END IF;
    IF session_uuid IS NULL THEN
        RETURN NULL;
    END IF;
    signature_value := CASE WHEN TG_OP = 'INSERT' THEN NEW.signature ELSE OLD.signature END;
    SELECT * INTO root_row FROM refresh_sessions WHERE id = session_uuid;
    IF NOT FOUND OR root_row.terminal_reason IS NOT NULL OR NOT root_row.lineage_valid THEN
        RETURN NULL; -- Cascading deletion and terminal invalidation never create authority.
    END IF;
    IF TG_OP = 'INSERT' THEN
        safe_change := signature_value = root_row.current_signature
            AND refresh_lineage_current_edge_valid(session_uuid);
    ELSIF TG_OP = 'UPDATE' THEN
        IF TG_TABLE_NAME = 'refresh_tokens' THEN
            safe_change := OLD.signature = NEW.signature AND OLD.session_id = NEW.session_id
                AND OLD.issued_at = NEW.issued_at AND OLD.expires_at = NEW.expires_at;
        ELSE
            safe_change := OLD.signature = NEW.signature AND OLD.session_id = NEW.session_id
                AND OLD.request_id = NEW.request_id AND OLD.agent_id = NEW.agent_id
                AND OLD.principal = NEW.principal AND OLD.client_id = NEW.client_id
                AND OLD.scope = NEW.scope AND OLD.created_at = NEW.created_at
                AND OLD.expires_at = NEW.expires_at AND OLD.email IS NOT DISTINCT FROM NEW.email
                AND OLD.display_name = NEW.display_name
                AND OLD.predecessor_signature IS NOT DISTINCT FROM NEW.predecessor_signature;
        END IF;
        -- Only the immediately preceding token can transition from unused to used,
        -- and only if both native and mirror agree with the issued successor.
        safe_change := safe_change AND OLD.used_at IS NULL AND NEW.used_at IS NOT NULL
            AND root_row.previous_signature = signature_value
            AND root_row.previous_consumed_at = NEW.used_at
            AND root_row.last_fresh_at = NEW.used_at
            AND refresh_lineage_current_edge_valid(session_uuid);
    END IF;
    IF safe_change IS NOT TRUE THEN
        UPDATE refresh_sessions SET lineage_valid = FALSE WHERE id = session_uuid AND lineage_valid;
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER refresh_native_lineage_proof
    AFTER INSERT OR UPDATE OR DELETE ON refresh_tokens
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION refresh_lineage_watch_child();
CREATE CONSTRAINT TRIGGER refresh_mirror_lineage_proof
    AFTER INSERT OR UPDATE OR DELETE ON refresh_token_sessions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION refresh_lineage_watch_child();

CREATE FUNCTION refresh_lineage_watch_root() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NOT refresh_lineage_current_edge_valid(NEW.id) OR NOT EXISTS (
            SELECT 1 FROM user_grants g WHERE g.id = NEW.original_grant_id
                AND g.agent_id = NEW.agent_id AND g.principal = NEW.principal
                AND (g.valid_until IS NULL OR g.valid_until > NEW.started_at)
        ) THEN
            UPDATE refresh_sessions SET lineage_valid = FALSE WHERE id = NEW.id AND lineage_valid;
        END IF;
    ELSIF OLD.lineage_valid AND OLD.terminal_reason IS NULL AND (
        OLD.original_grant_id IS DISTINCT FROM NEW.original_grant_id
        OR OLD.original_token_signature IS DISTINCT FROM NEW.original_token_signature
        OR OLD.agent_id IS DISTINCT FROM NEW.agent_id OR OLD.principal IS DISTINCT FROM NEW.principal
        OR OLD.client_id IS DISTINCT FROM NEW.client_id OR OLD.scope IS DISTINCT FROM NEW.scope
        OR OLD.email IS DISTINCT FROM NEW.email OR OLD.display_name IS DISTINCT FROM NEW.display_name
        OR OLD.started_at IS DISTINCT FROM NEW.started_at OR OLD.branch_key_id IS DISTINCT FROM NEW.branch_key_id
        OR NEW.retain_until < OLD.retain_until
        OR (OLD.current_signature = NEW.current_signature AND (
            OLD.previous_signature IS DISTINCT FROM NEW.previous_signature
            OR OLD.previous_consumed_at IS DISTINCT FROM NEW.previous_consumed_at
            OR OLD.last_fresh_at IS DISTINCT FROM NEW.last_fresh_at))
        OR (OLD.current_signature <> NEW.current_signature AND (
            NEW.previous_signature IS DISTINCT FROM OLD.current_signature
            OR NEW.previous_consumed_at IS DISTINCT FROM NEW.last_fresh_at
            OR NOT refresh_lineage_current_edge_valid(NEW.id)))
    ) THEN
        UPDATE refresh_sessions SET lineage_valid = FALSE WHERE id = NEW.id AND lineage_valid;
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER refresh_root_lineage_proof
    AFTER INSERT OR UPDATE ON refresh_sessions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION refresh_lineage_watch_root();

CREATE FUNCTION refresh_lineage_forbid_repair() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.lineage_valid = FALSE AND NEW.lineage_valid = TRUE THEN
        RAISE EXCEPTION 'broken refresh lineage cannot regain authority';
    END IF;
    IF OLD.terminal_reason IS NOT NULL AND (
        OLD.terminal_reason IS DISTINCT FROM NEW.terminal_reason
        OR OLD.revoked_at IS DISTINCT FROM NEW.revoked_at
        OR OLD.expired_at IS DISTINCT FROM NEW.expired_at
    ) THEN
        RAISE EXCEPTION 'terminal refresh authority cannot be restored or rewritten';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER refresh_lineage_proof_no_repair
    BEFORE UPDATE ON refresh_sessions
    FOR EACH ROW EXECUTE FUNCTION refresh_lineage_forbid_repair();
