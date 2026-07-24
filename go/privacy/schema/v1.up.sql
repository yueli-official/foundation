CREATE TABLE privacy_instances (
    instance_key TEXT PRIMARY KEY,
    schema_version BIGINT NOT NULL,
    catalog_version BIGINT NOT NULL,
    catalog_digest TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE privacy_definition_revisions (
    instance_key TEXT NOT NULL REFERENCES privacy_instances(instance_key) ON DELETE CASCADE,
    definition_kind TEXT NOT NULL,
    definition_key TEXT NOT NULL,
    revision BIGINT NOT NULL,
    digest TEXT NOT NULL,
    definition JSONB NOT NULL,
    PRIMARY KEY (instance_key, definition_kind, definition_key, revision)
);

CREATE TABLE privacy_command_receipts (
    instance_key TEXT NOT NULL REFERENCES privacy_instances(instance_key) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    command_kind TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    receipt JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, idempotency_key)
);

CREATE TABLE privacy_evidence_events (
    instance_key TEXT NOT NULL REFERENCES privacy_instances(instance_key) ON DELETE CASCADE,
    receipt_id TEXT NOT NULL,
    event_kind TEXT NOT NULL CHECK (event_kind IN ('consent', 'withdrawal', 'signal')),
    subject_owner TEXT NOT NULL,
    subject_kind TEXT NOT NULL,
    subject_value TEXT NOT NULL,
    purposes JSONB NOT NULL DEFAULT '[]'::jsonb,
    signal_key TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,
    receipt JSONB NOT NULL,
    PRIMARY KEY (instance_key, receipt_id)
);

CREATE INDEX privacy_evidence_subject_idx
    ON privacy_evidence_events(instance_key, subject_owner, subject_kind, subject_value, occurred_at DESC);

CREATE TABLE privacy_retention_items (
    instance_key TEXT NOT NULL REFERENCES privacy_instances(instance_key) ON DELETE CASCADE,
    item_id TEXT NOT NULL,
    record_dataset TEXT NOT NULL,
    record_value TEXT NOT NULL,
    rule_key TEXT NOT NULL,
    rule_revision BIGINT NOT NULL,
    review_at TIMESTAMPTZ NOT NULL,
    state TEXT NOT NULL,
    item JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, item_id)
);

CREATE INDEX privacy_retention_due_idx
    ON privacy_retention_items(instance_key, state, review_at, item_id);

CREATE TABLE privacy_rights_requests (
    instance_key TEXT NOT NULL REFERENCES privacy_instances(instance_key) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    operation TEXT NOT NULL,
    subject JSONB NOT NULL,
    phase TEXT NOT NULL,
    deadline TIMESTAMPTZ NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, request_id),
    UNIQUE (instance_key, idempotency_key)
);

CREATE TABLE privacy_owner_tasks (
    instance_key TEXT NOT NULL,
    request_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    owner_key TEXT NOT NULL,
    phase TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    lease_until TIMESTAMPTZ,
    command_fingerprint TEXT NOT NULL,
    command JSONB NOT NULL,
    terminal_receipt JSONB,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, task_id),
    FOREIGN KEY (instance_key, request_id)
        REFERENCES privacy_rights_requests(instance_key, request_id) ON DELETE CASCADE
);

CREATE INDEX privacy_owner_tasks_due_idx
    ON privacy_owner_tasks(instance_key, request_id, phase, next_attempt_at, task_id);

CREATE TABLE privacy_owner_receipts (
    instance_key TEXT NOT NULL,
    task_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    fingerprint TEXT NOT NULL,
    receipt JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, task_id, sequence),
    FOREIGN KEY (instance_key, task_id)
        REFERENCES privacy_owner_tasks(instance_key, task_id) ON DELETE CASCADE
);

CREATE TABLE privacy_host_commands (
    instance_key TEXT NOT NULL REFERENCES privacy_instances(instance_key) ON DELETE CASCADE,
    owner_key TEXT NOT NULL,
    task_id TEXT NOT NULL,
    command_fingerprint TEXT NOT NULL,
    command JSONB NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    terminal BOOLEAN NOT NULL DEFAULT FALSE,
    latest_receipt JSONB,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, owner_key, task_id)
);

CREATE TABLE privacy_host_receipts (
    instance_key TEXT NOT NULL,
    owner_key TEXT NOT NULL,
    task_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    fingerprint TEXT NOT NULL,
    receipt JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, owner_key, task_id, sequence),
    FOREIGN KEY (instance_key, owner_key, task_id)
        REFERENCES privacy_host_commands(instance_key, owner_key, task_id) ON DELETE CASCADE
);
