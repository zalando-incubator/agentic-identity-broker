-- After dropping the root/token evidence, no former anchored family can be
-- safely renewed by an old binary. Fence its mirrors and same-request descendants
-- without changing previously consumed timestamps or unrelated legacy rows.
UPDATE refresh_token_sessions AS legacy
SET used_at = clock_timestamp()
WHERE legacy.used_at IS NULL
  AND (
      legacy.session_id IS NOT NULL
      OR EXISTS (
          SELECT 1 FROM refresh_sessions AS root
          WHERE root.id::text = legacy.request_id AND root.agent_id = legacy.agent_id
      )
  );


ALTER TABLE refresh_token_sessions
    DROP COLUMN predecessor_signature,
    DROP COLUMN session_id;

ALTER TABLE refresh_sessions
    DROP CONSTRAINT refresh_sessions_original_token_fk,
    DROP CONSTRAINT refresh_sessions_current_token_fk;

DROP TABLE refresh_tokens;
DROP TABLE refresh_sessions;
DROP TABLE refresh_revocation_receipts;
DROP FUNCTION reject_refresh_revocation_receipt_change();
