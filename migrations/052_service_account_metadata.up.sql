SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE service_accounts IN ACCESS EXCLUSIVE MODE NOWAIT;

    ALTER TABLE service_accounts
        ADD COLUMN labels JSONB NOT NULL DEFAULT '{}',
        ADD COLUMN annotations JSONB NOT NULL DEFAULT '{}',
        ADD CONSTRAINT chk_service_accounts_labels_object CHECK (jsonb_typeof(labels) = 'object'),
        ADD CONSTRAINT chk_service_accounts_annotations_object CHECK (jsonb_typeof(annotations) = 'object');
END
$migration$;
