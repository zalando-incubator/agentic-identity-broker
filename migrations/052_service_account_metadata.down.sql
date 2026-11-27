SET LOCAL statement_timeout = '1s';

DO $migration$
BEGIN
    LOCK TABLE service_accounts IN ACCESS EXCLUSIVE MODE NOWAIT;

    ALTER TABLE service_accounts
        DROP CONSTRAINT chk_service_accounts_annotations_object,
        DROP CONSTRAINT chk_service_accounts_labels_object,
        DROP COLUMN annotations,
        DROP COLUMN labels;
END
$migration$;
