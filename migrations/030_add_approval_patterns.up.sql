-- Migration 030: Add glob pattern columns to tool_approvals
--
-- tool_pattern/params_pattern record which future invocations an approval decision covers.
-- Both are NOT NULL, so existing rows are backfilled to the exact coverage of the call they
-- were created for. The backfill fires trigger_tool_approvals_sync once per row; this is
-- expected and causes ExtProc to re-synchronize once.

ALTER TABLE tool_approvals ADD COLUMN tool_pattern VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE tool_approvals ADD COLUMN params_pattern JSONB NOT NULL DEFAULT '{}';

CREATE FUNCTION aib_canonical_json(v jsonb) RETURNS text
LANGUAGE plpgsql IMMUTABLE AS $fn$
DECLARE
    parts text[];
BEGIN
    CASE jsonb_typeof(v)
    WHEN 'object' THEN
        SELECT array_agg(to_jsonb(e.key)::text || ':' || aib_canonical_json(e.value)
                         ORDER BY e.key COLLATE "C")
          INTO parts FROM jsonb_each(v) AS e;
        RETURN '{' || COALESCE(array_to_string(parts, ','), '') || '}';
    WHEN 'array' THEN
        SELECT array_agg(aib_canonical_json(a.value) ORDER BY a.ord)
          INTO parts FROM jsonb_array_elements(v) WITH ORDINALITY AS a(value, ord);
        RETURN '[' || COALESCE(array_to_string(parts, ','), '') || ']';
    WHEN 'number' THEN
        RETURN trim_scale((v #>> '{}')::numeric)::text;
    WHEN 'null' THEN
        RETURN 'null';
    ELSE
        RETURN v::text;
    END CASE;
END;
$fn$;

CREATE FUNCTION aib_canonical_value(v jsonb) RETURNS text
LANGUAGE sql IMMUTABLE AS $fn$
    SELECT CASE jsonb_typeof(v)
        WHEN 'string' THEN v #>> '{}'
        ELSE aib_canonical_json(v)
    END
$fn$;

CREATE FUNCTION aib_glob_escape(t text) RETURNS text
LANGUAGE sql IMMUTABLE AS $fn$
    SELECT replace(replace(t, '\', '\\'), '*', '\*')
$fn$;

UPDATE tool_approvals
SET tool_pattern = tool_name,
    params_pattern = COALESCE((
        SELECT jsonb_object_agg(e.key, aib_glob_escape(aib_canonical_value(e.value)))
        FROM jsonb_each(arguments) AS e
    ), '{}'::jsonb);

DROP FUNCTION aib_glob_escape(text);
DROP FUNCTION aib_canonical_value(jsonb);
DROP FUNCTION aib_canonical_json(jsonb);

ALTER TABLE tool_approvals ALTER COLUMN tool_pattern DROP DEFAULT;
