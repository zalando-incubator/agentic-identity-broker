-- Without the durable proof, neither old nor new binaries may renew an anchored
-- family. Keep the legacy bridge/table for rolling deployments, but fence its
-- anchored rows and any old-only descendants before removing the proof.
DROP TRIGGER refresh_lineage_proof_no_repair ON refresh_sessions;
DROP TRIGGER refresh_root_lineage_proof ON refresh_sessions;
DROP TRIGGER refresh_mirror_lineage_proof ON refresh_token_sessions;
DROP TRIGGER refresh_native_lineage_proof ON refresh_tokens;

UPDATE refresh_token_sessions bridge SET used_at = clock_timestamp()
WHERE bridge.used_at IS NULL AND (
    bridge.session_id IS NOT NULL OR EXISTS (
        SELECT 1 FROM refresh_sessions root
        WHERE root.id::text = bridge.request_id AND root.agent_id = bridge.agent_id
    )
);

DROP FUNCTION refresh_lineage_forbid_repair();
DROP FUNCTION refresh_lineage_watch_root();
DROP FUNCTION refresh_lineage_watch_child();
DROP FUNCTION refresh_lineage_current_edge_valid(UUID);
ALTER TABLE refresh_sessions DROP COLUMN lineage_valid;
