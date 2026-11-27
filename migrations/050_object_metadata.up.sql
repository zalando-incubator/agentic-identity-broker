ALTER TABLE agents
    ADD COLUMN labels JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN annotations JSONB NOT NULL DEFAULT '{}',
    ADD CONSTRAINT chk_agents_labels_object CHECK (jsonb_typeof(labels) = 'object'),
    ADD CONSTRAINT chk_agents_annotations_object CHECK (jsonb_typeof(annotations) = 'object');

ALTER TABLE thirdparty_oauth2_services
    ADD COLUMN labels JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN annotations JSONB NOT NULL DEFAULT '{}',
    ADD CONSTRAINT chk_thirdparty_oauth2_services_labels_object CHECK (jsonb_typeof(labels) = 'object'),
    ADD CONSTRAINT chk_thirdparty_oauth2_services_annotations_object CHECK (jsonb_typeof(annotations) = 'object');

ALTER TABLE permission_sets
    ADD COLUMN labels JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN annotations JSONB NOT NULL DEFAULT '{}',
    ADD CONSTRAINT chk_permission_sets_labels_object CHECK (jsonb_typeof(labels) = 'object'),
    ADD CONSTRAINT chk_permission_sets_annotations_object CHECK (jsonb_typeof(annotations) = 'object');
