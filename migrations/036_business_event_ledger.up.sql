CREATE TABLE public.business_events (
    recorded_at timestamptz NOT NULL,
    id uuid NOT NULL,
    occurred_at timestamptz NOT NULL,
    type text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('success', 'failure', 'denied', 'pending')),
    subject text,
    envelope jsonb NOT NULL,
    PRIMARY KEY (recorded_at, id),
    CONSTRAINT business_event_projection CHECK ((
        jsonb_typeof(envelope) = 'object'
        AND envelope->'id' = to_jsonb(id::text)
        AND envelope->'type' = to_jsonb(type)
        AND envelope->'outcome' = to_jsonb(outcome)
        AND envelope->'subject' = COALESCE(to_jsonb(subject), 'null'::jsonb)
        AND jsonb_typeof(envelope->'occurred_at') = 'string'
        AND jsonb_typeof(envelope->'recorded_at') = 'string'
        AND envelope->>'occurred_at' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]{1,6})?(Z|[+-][0-9]{2}:[0-9]{2})$'
        AND envelope->>'recorded_at' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]{1,6})?(Z|[+-][0-9]{2}:[0-9]{2})$'
        AND (envelope->>'occurred_at')::timestamptz = occurred_at
        AND (envelope->>'recorded_at')::timestamptz = recorded_at
    ) IS TRUE)
) PARTITION BY RANGE (recorded_at);

CREATE INDEX business_events_subject_type_outcome_occurrence
    ON public.business_events (subject, type, outcome, occurred_at, id);
CREATE INDEX business_events_subject_occurrence
    ON public.business_events (subject, occurred_at, id);
CREATE INDEX business_events_occurrence
    ON public.business_events (occurred_at, id);

CREATE TABLE public.business_event_delivery_pending (
    recorded_at timestamptz NOT NULL,
    event_id uuid NOT NULL,
    next_attempt_at timestamptz NOT NULL,
    PRIMARY KEY (recorded_at, event_id)
) PARTITION BY RANGE (recorded_at);

CREATE INDEX business_event_delivery_due
    ON public.business_event_delivery_pending (next_attempt_at, recorded_at, event_id);

CREATE TABLE public.business_event_policy (
    singleton boolean PRIMARY KEY CHECK (singleton),
    retention_microseconds bigint NOT NULL CHECK (retention_microseconds > 0)
);
INSERT INTO public.business_event_policy VALUES (true, 7776000000000);

ALTER TABLE public.user_grants ADD COLUMN expiration_recorded_for timestamptz;
ALTER TABLE public.tool_approvals ADD COLUMN expiration_recorded_for timestamptz;

CREATE INDEX tool_approvals_unrecorded_expiry_by_principal
    ON public.tool_approvals (principal, expires_at, id)
    WHERE status = 'pending' AND expiration_recorded_for IS DISTINCT FROM expires_at;

CREATE FUNCTION public.business_event_reject_update() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $fn$
BEGIN
    RAISE EXCEPTION 'business events are immutable' USING ERRCODE = '55000';
END;
$fn$;

CREATE TRIGGER business_event_immutable BEFORE UPDATE ON public.business_events
    FOR EACH ROW EXECUTE FUNCTION public.business_event_reject_update();

CREATE FUNCTION public.business_event_create_partition_pair(lower_bound timestamptz) RETURNS void
LANGUAGE plpgsql SECURITY INVOKER SET search_path = pg_catalog SET timezone = 'UTC' AS $fn$
DECLARE
    upper_bound timestamptz;
    parent_name text;
    child_name text;
    suffix text;
    child oid;
    expected_bound text;
    recipient record;
BEGIN
    IF lower_bound IS NULL OR NOT isfinite(lower_bound)
        OR lower_bound <> date_bin(interval '6 hours', lower_bound, timestamptz '2000-01-01 00:00:00+00') THEN
        RAISE EXCEPTION 'invalid business event partition boundary' USING ERRCODE = '22023';
    END IF;
    PERFORM pg_advisory_xact_lock(1095320140, 0);
    upper_bound := lower_bound + interval '6 hours';
    suffix := to_char(lower_bound AT TIME ZONE 'UTC', 'YYYYMMDD_HH24');
    expected_bound := format('FOR VALUES FROM (%L) TO (%L)', lower_bound::text, upper_bound::text);
    FOREACH parent_name IN ARRAY ARRAY['business_events', 'business_event_delivery_pending'] LOOP
        child_name := parent_name || '_' || suffix;
        child := to_regclass(format('public.%I', child_name));
        IF child IS NULL THEN
            EXECUTE format('CREATE TABLE public.%I PARTITION OF public.%I FOR VALUES FROM (%L) TO (%L)',
                child_name, parent_name, lower_bound, upper_bound);
            child := to_regclass(format('public.%I', child_name));
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
            WHERE i.inhrelid = child AND i.inhparent = to_regclass(format('public.%I', parent_name))
                AND pg_get_expr(c.relpartbound, c.oid) = expected_bound
        ) THEN
            RAISE EXCEPTION 'invalid business event partition metadata' USING ERRCODE = '55000';
        END IF;
        FOR recipient IN
            SELECT DISTINCT a.grantee FROM pg_class c,
                LATERAL aclexplode(COALESCE(c.relacl, acldefault('r', c.relowner))) a
            WHERE c.oid = child AND a.grantee <> c.relowner
        LOOP
            IF recipient.grantee = 0 THEN
                EXECUTE format('REVOKE ALL ON TABLE public.%I FROM PUBLIC', child_name);
            ELSE
                EXECUTE format('REVOKE ALL ON TABLE public.%I FROM %I', child_name, pg_get_userbyid(recipient.grantee));
            END IF;
        END LOOP;
    END LOOP;
END;
$fn$;

CREATE FUNCTION public.business_event_provision_partitions() RETURNS void
LANGUAGE plpgsql SECURITY INVOKER SET search_path = pg_catalog SET timezone = 'UTC' AS $fn$
DECLARE
    first_bound timestamptz;
    window_index integer;
BEGIN
    PERFORM pg_advisory_xact_lock(1095320140, 0);
    first_bound := date_trunc('day', clock_timestamp() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
    FOR window_index IN 0..31 LOOP
        PERFORM public.business_event_create_partition_pair(first_bound + window_index * interval '6 hours');
    END LOOP;
END;
$fn$;

CREATE FUNCTION public.business_event_maintain_partitions() RETURNS void
LANGUAGE plpgsql SECURITY INVOKER SET search_path = pg_catalog SET timezone = 'UTC'
SET lock_timeout = '5s' AS $fn$
DECLARE
    sweep_now timestamptz;
    retention_us bigint;
    cutoff timestamptz;
    child record;
    bounds text[];
    lower_bound timestamptz;
    upper_bound timestamptz;
    expected_bound text;
    suffix text;
    expired_pairs timestamptz[] := ARRAY[]::timestamptz[];
BEGIN
    IF current_setting('statement_timeout')::interval NOT BETWEEN interval '1 millisecond' AND interval '30 seconds' THEN
        RAISE EXCEPTION 'set statement_timeout between 1ms and 30s before calling ledger maintenance' USING ERRCODE = '55000';
    END IF;
    PERFORM pg_advisory_xact_lock(1095320140, 0);
    sweep_now := clock_timestamp();
    SELECT retention_microseconds INTO STRICT retention_us FROM public.business_event_policy WHERE singleton;
    IF retention_us <= 0 THEN
        RAISE EXCEPTION 'invalid business event retention policy' USING ERRCODE = '22023';
    END IF;
    -- Split whole seconds to preserve microseconds above the float integer limit.
    cutoff := sweep_now - ((retention_us / 1000000) * interval '1 second'
        + (retention_us % 1000000) * interval '1 microsecond');
    PERFORM public.business_event_provision_partitions();
    FOR child IN
        SELECT c.oid, c.relname, n.nspname, p.relname AS parent_name,
            pg_get_expr(c.relpartbound, c.oid) AS partition_bound
        FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
        JOIN pg_namespace n ON n.oid = c.relnamespace JOIN pg_class p ON p.oid = i.inhparent
        WHERE i.inhparent IN ('public.business_events'::regclass, 'public.business_event_delivery_pending'::regclass)
        ORDER BY c.relname
    LOOP
        bounds := regexp_match(child.partition_bound, '^FOR VALUES FROM \(''([^'']+)''\) TO \(''([^'']+)''\)$');
        IF bounds IS NULL OR child.nspname <> 'public' THEN
            RAISE EXCEPTION 'invalid business event partition metadata' USING ERRCODE = '55000';
        END IF;
        lower_bound := bounds[1]::timestamptz;
        upper_bound := bounds[2]::timestamptz;
        suffix := to_char(lower_bound AT TIME ZONE 'UTC', 'YYYYMMDD_HH24');
        expected_bound := format('FOR VALUES FROM (%L) TO (%L)', lower_bound::text, upper_bound::text);
        IF NOT isfinite(lower_bound) OR NOT isfinite(upper_bound)
            OR lower_bound <> date_bin(interval '6 hours', lower_bound, timestamptz '2000-01-01 00:00:00+00')
            OR upper_bound <> lower_bound + interval '6 hours'
            OR child.relname <> child.parent_name || '_' || suffix
            OR NOT EXISTS (
                SELECT 1 FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
                WHERE i.inhparent = 'public.business_events'::regclass
                    AND i.inhrelid = to_regclass(format('public.%I', 'business_events_' || suffix))
                    AND pg_get_expr(c.relpartbound, c.oid) = expected_bound
            ) OR NOT EXISTS (
                SELECT 1 FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
                WHERE i.inhparent = 'public.business_event_delivery_pending'::regclass
                    AND i.inhrelid = to_regclass(format('public.%I', 'business_event_delivery_pending_' || suffix))
                    AND pg_get_expr(c.relpartbound, c.oid) = expected_bound
            ) THEN
            RAISE EXCEPTION 'invalid business event partition metadata' USING ERRCODE = '55000';
        END IF;
        IF child.parent_name = 'business_events' AND upper_bound <= cutoff THEN
            expired_pairs := array_append(expired_pairs, lower_bound);
        END IF;
    END LOOP;
    FOREACH lower_bound IN ARRAY expired_pairs LOOP
        suffix := to_char(lower_bound AT TIME ZONE 'UTC', 'YYYYMMDD_HH24');
        EXECUTE format('DROP TABLE public.%I, public.%I', 'business_event_delivery_pending_' || suffix,
            'business_events_' || suffix);
    END LOOP;
END;
$fn$;

CREATE FUNCTION public.business_event_erase_subject(selected_subject text) RETURNS bigint
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
SET lock_timeout = '5s' AS $fn$
DECLARE
    subject_bytes bytea;
    subject_hash bigint := 2166136261;
    byte_index integer;
    deleted_count bigint;
BEGIN
    IF selected_subject IS NULL OR btrim(selected_subject) = '' THEN
        RAISE EXCEPTION 'erasure requires an exact nonempty subject' USING ERRCODE = '22023';
    END IF;
    IF current_setting('transaction_isolation') <> 'read committed' THEN
        RAISE EXCEPTION 'business event erasure requires READ COMMITTED isolation' USING ERRCODE = '25001';
    END IF;
    IF current_setting('statement_timeout')::interval NOT BETWEEN interval '1 millisecond' AND interval '30 seconds' THEN
        RAISE EXCEPTION 'set statement_timeout between 1ms and 30s before calling ledger erasure' USING ERRCODE = '55000';
    END IF;
    PERFORM pg_advisory_xact_lock_shared(1095320140, 0);
    -- Match the broker's existing FNV-1a advisory key, including UTF-8 bytes.
    subject_bytes := convert_to(selected_subject, 'UTF8');
    FOR byte_index IN 0..octet_length(subject_bytes) - 1 LOOP
        subject_hash := ((subject_hash # get_byte(subject_bytes, byte_index)) * 16777619) % 4294967296;
    END LOOP;
    IF subject_hash >= 2147483648 THEN
        subject_hash := subject_hash - 4294967296;
    END IF;
    PERFORM pg_advisory_xact_lock(1095320147, subject_hash::integer);
    PERFORM 1 FROM public.business_event_delivery_pending d JOIN public.business_events e
        ON d.recorded_at = e.recorded_at AND d.event_id = e.id
        WHERE e.subject = selected_subject ORDER BY d.recorded_at, d.event_id FOR UPDATE OF d;
    DELETE FROM public.business_event_delivery_pending d USING public.business_events e
        WHERE d.recorded_at = e.recorded_at AND d.event_id = e.id AND e.subject = selected_subject;
    DELETE FROM public.business_events WHERE subject = selected_subject;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$fn$;

REVOKE ALL ON FUNCTION public.business_event_reject_update() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.business_event_create_partition_pair(timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.business_event_provision_partitions() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.business_event_maintain_partitions() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.business_event_erase_subject(text) FROM PUBLIC;

DO $privileges$
DECLARE
    recipient record;
    function_oid oid;
BEGIN
    FOR recipient IN
        SELECT DISTINCT a.grantee FROM pg_class c,
            LATERAL aclexplode(COALESCE(c.relacl, acldefault('r', c.relowner))) a
        WHERE c.oid = 'public.business_events'::regclass AND a.grantee <> c.relowner
    LOOP
        IF recipient.grantee = 0 THEN
            REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.business_events FROM PUBLIC;
        ELSE
            EXECUTE format('REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.business_events FROM %I',
                pg_get_userbyid(recipient.grantee));
        END IF;
    END LOOP;
    FOR function_oid IN
        SELECT p.oid FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
        WHERE n.nspname = 'public' AND p.proname IN ('business_event_reject_update',
            'business_event_create_partition_pair', 'business_event_provision_partitions',
            'business_event_maintain_partitions', 'business_event_erase_subject')
    LOOP
        FOR recipient IN
            SELECT DISTINCT a.grantee FROM pg_proc p,
                LATERAL aclexplode(COALESCE(p.proacl, acldefault('f', p.proowner))) a
            WHERE p.oid = function_oid AND a.grantee <> p.proowner AND a.grantee <> 0
        LOOP
            EXECUTE format('REVOKE ALL ON FUNCTION %s FROM %I', function_oid::regprocedure,
                pg_get_userbyid(recipient.grantee));
        END LOOP;
    END LOOP;
END;
$privileges$;

SELECT public.business_event_provision_partitions();
