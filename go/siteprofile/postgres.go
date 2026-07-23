package siteprofile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type postgresDB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type PostgresStore struct {
	db     postgresDB
	prefix string
}

func NewPostgresStore(db *sql.DB, prefix string) (*PostgresStore, error) {
	if db == nil {
		return nil, errors.New("siteprofile: PostgreSQL database is required")
	}
	return newPostgresStore(db, prefix)
}

func newPostgresStore(db postgresDB, prefix string) (*PostgresStore, error) {
	if prefix == "" {
		prefix = DefaultPostgresPrefix
	}
	if !postgresPrefixPattern.MatchString(prefix) {
		return nil, fmt.Errorf("siteprofile: PostgreSQL prefix must match %s", postgresPrefixPattern)
	}
	return &PostgresStore{db: db, prefix: prefix}, nil
}

func (s *PostgresStore) Bind(tx *sql.Tx) (*PostgresStore, error) {
	if tx == nil {
		return nil, errors.New("siteprofile: PostgreSQL transaction is required")
	}
	return newPostgresStore(tx, s.prefix)
}

func (s *PostgresStore) Load(ctx context.Context) (StoredState, bool, error) {
	query := fmt.Sprintf(`
SELECT revision, schema_version, document::text, document_digest, updated_at
FROM %sstate
WHERE id = 1`, s.prefix)
	var revision, schemaVersion uint64
	var document []byte
	var digest string
	var updatedAt time.Time
	err := s.db.QueryRowContext(ctx, query).Scan(&revision, &schemaVersion, &document, &digest, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredState{}, false, nil
	}
	if err != nil {
		return StoredState{}, false, fmt.Errorf("siteprofile: load PostgreSQL state: %w", err)
	}
	return StoredState{
		Document: document, Digest: Digest(digest), Revision: Revision(revision),
		SchemaVersion: schemaVersion, UpdatedAt: updatedAt.UTC(),
	}, true, nil
}

func (s *PostgresStore) CompareAndSwap(ctx context.Context, expected Revision, next StoredState) (bool, Revision, error) {
	var (
		result sql.Result
		err    error
	)
	if expected == 0 {
		query := fmt.Sprintf(`
INSERT INTO %sstate (id, revision, schema_version, document, document_digest, updated_at)
VALUES (1, $1, $2, $3::jsonb, $4, $5)
ON CONFLICT (id) DO NOTHING`, s.prefix)
		result, err = s.db.ExecContext(ctx, query, next.Revision, next.SchemaVersion, next.Document, next.Digest, next.UpdatedAt)
	} else {
		query := fmt.Sprintf(`
UPDATE %sstate
SET revision = $1, schema_version = $2, document = $3::jsonb, document_digest = $4, updated_at = $5
WHERE id = 1 AND revision = $6`, s.prefix)
		result, err = s.db.ExecContext(
			ctx, query, next.Revision, next.SchemaVersion, next.Document, next.Digest, next.UpdatedAt, expected,
		)
	}
	if err != nil {
		return false, 0, fmt.Errorf("siteprofile: compare and swap PostgreSQL state: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, 0, fmt.Errorf("siteprofile: inspect PostgreSQL state update: %w", err)
	}
	if affected == 1 {
		return true, next.Revision, nil
	}
	actual, found, err := s.currentRevision(ctx)
	if err != nil {
		return false, 0, err
	}
	if !found {
		actual = 0
	}
	return false, actual, nil
}

func (s *PostgresStore) currentRevision(ctx context.Context) (Revision, bool, error) {
	query := fmt.Sprintf(`SELECT revision FROM %sstate WHERE id = 1`, s.prefix)
	var revision uint64
	err := s.db.QueryRowContext(ctx, query).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("siteprofile: read PostgreSQL revision: %w", err)
	}
	return Revision(revision), true, nil
}
