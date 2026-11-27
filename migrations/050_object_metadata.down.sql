ALTER TABLE permission_sets
    DROP CONSTRAINT chk_permission_sets_annotations_object,
    DROP CONSTRAINT chk_permission_sets_labels_object,
    DROP COLUMN annotations,
    DROP COLUMN labels;

ALTER TABLE thirdparty_oauth2_services
    DROP CONSTRAINT chk_thirdparty_oauth2_services_annotations_object,
    DROP CONSTRAINT chk_thirdparty_oauth2_services_labels_object,
    DROP COLUMN annotations,
    DROP COLUMN labels;

ALTER TABLE agents
    DROP CONSTRAINT chk_agents_annotations_object,
    DROP CONSTRAINT chk_agents_labels_object,
    DROP COLUMN annotations,
    DROP COLUMN labels;
