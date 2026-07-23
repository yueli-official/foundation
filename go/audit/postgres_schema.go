package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const PostgresSchemaVersion = 1

type WrittenPostgresMigration struct {
	UpPath   string
	DownPath string
	Digest   string
}

func PostgresSchemaUp() string {
	return strings.TrimSpace(`
CREATE TABLE audit_instances (
    instance_key TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL,
    consumer TEXT NOT NULL,
    definition_version BIGINT NOT NULL,
    definition_digest TEXT NOT NULL,
    head_sequence BIGINT NOT NULL DEFAULT 0,
    head_digest TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE audit_action_definitions (
    instance_key TEXT NOT NULL REFERENCES audit_instances(instance_key) ON DELETE RESTRICT,
    action_name TEXT NOT NULL,
    action_version INTEGER NOT NULL,
    definition_digest TEXT NOT NULL,
    definition JSONB NOT NULL,
    registered_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, action_name, action_version)
);

CREATE TABLE audit_events (
    instance_key TEXT NOT NULL REFERENCES audit_instances(instance_key) ON DELETE RESTRICT,
    id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    envelope_version INTEGER NOT NULL,
    source JSONB NOT NULL,
    action_name TEXT NOT NULL,
    action_version INTEGER NOT NULL,
    category TEXT NOT NULL,
    actor_kind TEXT NOT NULL,
    actor_id TEXT,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    outcome_kind TEXT NOT NULL,
    outcome_reason TEXT,
    correlation JSONB NOT NULL,
    evidence JSONB NOT NULL,
    retention_class TEXT NOT NULL,
    definition_digest TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    previous_digest TEXT,
    digest TEXT NOT NULL,
    PRIMARY KEY (instance_key, sequence),
    UNIQUE (instance_key, id),
    FOREIGN KEY (instance_key, action_name, action_version)
        REFERENCES audit_action_definitions(instance_key, action_name, action_version) ON DELETE RESTRICT
);

CREATE TABLE audit_event_receipts (
    instance_key TEXT NOT NULL REFERENCES audit_instances(instance_key) ON DELETE RESTRICT,
    id TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    event_digest TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    purged_at TIMESTAMPTZ,
    PRIMARY KEY (instance_key, id),
    UNIQUE (instance_key, sequence)
);

CREATE INDEX audit_events_action_search
    ON audit_events(instance_key, action_name, action_version, sequence DESC);
CREATE INDEX audit_events_actor_search
    ON audit_events(instance_key, actor_kind, actor_id, sequence DESC);
CREATE INDEX audit_events_target_search
    ON audit_events(instance_key, target_type, target_id, sequence DESC);
CREATE INDEX audit_events_outcome_search
    ON audit_events(instance_key, outcome_kind, sequence DESC);
CREATE INDEX audit_events_occurred_search
    ON audit_events(instance_key, occurred_at DESC, sequence DESC);
CREATE INDEX audit_events_correlation_search
    ON audit_events(instance_key, ((correlation->>'requestId')), sequence DESC)
    WHERE correlation->>'requestId' IS NOT NULL;
CREATE INDEX audit_events_trace_search
    ON audit_events(instance_key, ((correlation->>'traceId')), sequence DESC)
    WHERE correlation->>'traceId' IS NOT NULL;

CREATE TABLE audit_legal_holds (
    instance_key TEXT NOT NULL REFERENCES audit_instances(instance_key) ON DELETE RESTRICT,
    id TEXT NOT NULL,
    selection JSONB NOT NULL,
    reason TEXT NOT NULL,
    placed_by JSONB NOT NULL,
    placed_at TIMESTAMPTZ NOT NULL,
    release_reason TEXT,
    released_by JSONB,
    released_at TIMESTAMPTZ,
    PRIMARY KEY (instance_key, id)
);

CREATE TABLE audit_retention_receipts (
    instance_key TEXT NOT NULL REFERENCES audit_instances(instance_key) ON DELETE RESTRICT,
    id TEXT NOT NULL,
    as_of TIMESTAMPTZ NOT NULL,
    deleted_count BIGINT NOT NULL,
    deleted_ranges JSONB NOT NULL,
    archive_manifest JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (instance_key, id)
);

CREATE TABLE audit_mirror_outbox (
    instance_key TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    event JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL,
    delivered_at TIMESTAMPTZ,
    last_error_code TEXT,
    PRIMARY KEY (instance_key, sequence),
    FOREIGN KEY (instance_key, sequence)
        REFERENCES audit_events(instance_key, sequence) ON DELETE CASCADE
);
`)
}

func PostgresSchemaDown() string {
	return strings.TrimSpace(`
DROP TABLE IF EXISTS audit_mirror_outbox;
DROP TABLE IF EXISTS audit_retention_receipts;
DROP TABLE IF EXISTS audit_legal_holds;
DROP TABLE IF EXISTS audit_event_receipts;
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS audit_action_definitions;
DROP TABLE IF EXISTS audit_instances;
`)
}

func PostgresMigration(version int, up bool) (string, error) {
	if version != PostgresSchemaVersion {
		return "", fmt.Errorf("audit: unsupported PostgreSQL schema version %d", version)
	}
	if up {
		return PostgresSchemaUp() + "\n", nil
	}
	return PostgresSchemaDown() + "\n", nil
}

func PostgresSchemaDigest(version int) (string, error) {
	up, err := PostgresMigration(version, true)
	if err != nil {
		return "", err
	}
	down, err := PostgresMigration(version, false)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(up), []byte(down)...))
	return hex.EncodeToString(sum[:]), nil
}

func WritePostgresMigration(directory, name string, version int) (WrittenPostgresMigration, error) {
	if strings.TrimSpace(directory) == "" {
		return WrittenPostgresMigration{}, errors.New("audit: migration directory is required")
	}
	if strings.TrimSpace(name) == "" || filepath.Base(name) != name {
		return WrittenPostgresMigration{}, errors.New("audit: migration name must be a base filename")
	}
	up, err := PostgresMigration(version, true)
	if err != nil {
		return WrittenPostgresMigration{}, err
	}
	down, err := PostgresMigration(version, false)
	if err != nil {
		return WrittenPostgresMigration{}, err
	}
	digest, err := PostgresSchemaDigest(version)
	if err != nil {
		return WrittenPostgresMigration{}, err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return WrittenPostgresMigration{}, fmt.Errorf("audit: create migration directory: %w", err)
	}
	header := fmt.Sprintf(
		"-- Code generated by foundation audit; DO NOT EDIT.\n-- audit-schema-version: %d\n-- audit-schema-digest: %s\n\n",
		version, digest,
	)
	result := WrittenPostgresMigration{
		UpPath:   filepath.Join(directory, name+".up.sql"),
		DownPath: filepath.Join(directory, name+".down.sql"),
		Digest:   digest,
	}
	if err := writeImmutablePostgresMigration(result.UpPath, []byte(header+up)); err != nil {
		return WrittenPostgresMigration{}, err
	}
	if err := writeImmutablePostgresMigration(result.DownPath, []byte(header+down)); err != nil {
		return WrittenPostgresMigration{}, err
	}
	return result, nil
}

func writeImmutablePostgresMigration(path string, content []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if string(existing) == string(content) {
			return nil
		}
		return fmt.Errorf("audit: migration drift at %s", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("audit: read migration %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("audit: write migration %s: %w", path, err)
	}
	return nil
}
