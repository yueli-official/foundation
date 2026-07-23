package urllifecycle

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"
)

type PostgresOptions struct {
	DB          *sql.DB
	InstanceKey string
	Prefix      string
	Clock       func() time.Time
}

type PostgresAdapter struct {
	db          *sql.DB
	tx          *sql.Tx
	catalog     *Catalog
	instanceKey string
	prefix      string
	clock       func() time.Time
	created     bool
}

var _ Module = (*PostgresAdapter)(nil)

func NewPostgres(
	ctx context.Context,
	catalog *Catalog,
	options PostgresOptions,
) (*PostgresAdapter, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	if options.DB == nil {
		return nil, invalid("db", "is required")
	}
	instanceKey := strings.TrimSpace(options.InstanceKey)
	if instanceKey == "" || len(instanceKey) > 200 || strings.ContainsRune(instanceKey, '\x00') {
		return nil, invalid("instance_key", "is invalid")
	}
	prefix := options.Prefix
	if prefix == "" {
		prefix = DefaultPostgresPrefix
	}
	if !postgresPrefixPattern.MatchString(prefix) {
		return nil, invalid("prefix", "is invalid")
	}
	if err := options.DB.PingContext(ctx); err != nil {
		return nil, unavailable("ping database", err)
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	adapter := &PostgresAdapter{
		db: options.DB, catalog: catalog, instanceKey: instanceKey,
		prefix: prefix, clock: clock,
	}
	tx, err := options.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable("begin bootstrap", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := clock().UTC()
	result, err := tx.ExecContext(ctx, `
INSERT INTO `+adapter.table("instances")+` (
    instance_key, schema_version, catalog_version, catalog_digest,
    revision, created_at, updated_at
)
VALUES ($1, $2, $3, $4, 0, $5, $5)
ON CONFLICT (instance_key) DO NOTHING`,
		instanceKey, CurrentPostgresSchemaVersion, catalog.Version(), catalog.Digest(), now,
	)
	if err != nil {
		return nil, unavailable("bootstrap instance", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, unavailable("read bootstrap result", err)
	}
	var schemaVersion, catalogVersion uint64
	var digest string
	if err := tx.QueryRowContext(ctx, `
SELECT schema_version, catalog_version, catalog_digest
FROM `+adapter.table("instances")+`
WHERE instance_key = $1
FOR UPDATE`, instanceKey).Scan(&schemaVersion, &catalogVersion, &digest); err != nil {
		return nil, unavailable("load instance", err)
	}
	if schemaVersion != uint64(CurrentPostgresSchemaVersion) {
		return nil, &Error{
			Kind: ErrorUnavailable, Field: "schema_version",
			Message: fmt.Sprintf("database has %d, module requires %d", schemaVersion, CurrentPostgresSchemaVersion),
		}
	}
	if catalogVersion != catalog.Version() || Digest(digest) != catalog.Digest() {
		return nil, &Error{
			Kind: ErrorConflict, Field: "catalog",
			Message: "database catalog version or digest does not match the compiled definition",
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, unavailable("commit bootstrap", err)
	}
	adapter.created = affected == 1
	return adapter, nil
}

func (adapter *PostgresAdapter) InstanceWasCreated() bool {
	return adapter != nil && adapter.created
}

// Bind returns an Adapter that uses a caller-owned transaction. The returned
// Adapter never commits or rolls back and must not outlive tx.
func (adapter *PostgresAdapter) Bind(tx *sql.Tx) (*PostgresAdapter, error) {
	if adapter == nil || tx == nil {
		return nil, invalid("tx", "is required")
	}
	bound := *adapter
	bound.tx = tx
	bound.created = false
	return &bound, nil
}

func (adapter *PostgresAdapter) Resolve(ctx context.Context, lookup Lookup) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	normalized, err := adapter.catalog.normalizeLookup(lookup)
	if err != nil {
		return Resolution{}, err
	}
	if adapter.tx != nil {
		return adapter.resolvePostgres(ctx, adapter.tx, normalized, adapter.clock().UTC())
	}
	tx, err := adapter.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return Resolution{}, unavailable("begin resolve", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := adapter.resolvePostgres(ctx, tx, normalized, adapter.clock().UTC())
	if err != nil {
		return Resolution{}, err
	}
	if err := tx.Commit(); err != nil {
		return Resolution{}, unavailable("commit resolve", err)
	}
	return result, nil
}

func (adapter *PostgresAdapter) Preview(ctx context.Context, set ChangeSet) (Plan, error) {
	var plan Plan
	err := adapter.withRead(ctx, func(tx *sql.Tx) error {
		state, err := adapter.loadState(ctx, tx, false)
		if err != nil {
			return err
		}
		result, err := planTransition(adapter.catalog, state, set, adapter.clock().UTC())
		plan = result.plan
		return err
	})
	return plan, err
}

func (adapter *PostgresAdapter) Apply(ctx context.Context, set ChangeSet, options ApplyOptions) (Receipt, error) {
	var receipt Receipt
	err := adapter.withWrite(ctx, func(tx *sql.Tx) error {
		current, err := adapter.loadState(ctx, tx, true)
		if err != nil {
			return err
		}
		result, err := planTransition(adapter.catalog, current, set, adapter.clock().UTC())
		if err != nil {
			return err
		}
		if options.Guard != nil &&
			(options.Guard.BaseRevision != result.plan.BaseRevision ||
				options.Guard.IntentDigest != result.plan.IntentDigest) {
			return &Error{Kind: ErrorStaleRevision, Field: "guard", Message: "preview guard no longer matches"}
		}
		if result.replay == nil {
			if err := adapter.persistState(ctx, tx, current, result.next); err != nil {
				return err
			}
		}
		receipt = result.receipt
		if result.replay != nil {
			receipt.Replay = true
		}
		return nil
	})
	return receipt, err
}

func (adapter *PostgresAdapter) Inspect(ctx context.Context, query InspectQuery) (Inspection, error) {
	var result Inspection
	err := adapter.withRead(ctx, func(tx *sql.Tx) error {
		state, err := adapter.loadState(ctx, tx, false)
		if err != nil {
			return err
		}
		result, err = inspectState(adapter.catalog, state, query, adapter.clock().UTC())
		return err
	})
	return result, err
}

func (adapter *PostgresAdapter) List(ctx context.Context, query ListQuery) (InspectionPage, error) {
	var result InspectionPage
	err := adapter.withRead(ctx, func(tx *sql.Tx) error {
		state, err := adapter.loadState(ctx, tx, false)
		if err != nil {
			return err
		}
		result, err = listState(adapter.catalog, state, query, adapter.clock().UTC())
		return err
	})
	return result, err
}

func (adapter *PostgresAdapter) History(ctx context.Context, query HistoryQuery) (HistoryPage, error) {
	var result HistoryPage
	err := adapter.withRead(ctx, func(tx *sql.Tx) error {
		state, err := adapter.loadState(ctx, tx, false)
		if err != nil {
			return err
		}
		result = historyState(state, query, adapter.catalog.limits.MaxPageSize)
		return nil
	})
	return result, err
}

func (adapter *PostgresAdapter) Export(ctx context.Context, query ExportQuery, writer io.Writer) (ArchiveManifest, error) {
	var result ArchiveManifest
	err := adapter.withRead(ctx, func(tx *sql.Tx) error {
		state, err := adapter.loadState(ctx, tx, false)
		if err != nil {
			return err
		}
		result, err = exportState(adapter.catalog, state, query, writer)
		return err
	})
	return result, err
}

func (adapter *PostgresAdapter) VerifyArchive(ctx context.Context, reader io.Reader) (ArchiveReport, error) {
	if err := ctx.Err(); err != nil {
		return ArchiveReport{}, err
	}
	return verifyArchive(adapter.catalog, reader)
}

func (adapter *PostgresAdapter) Restore(ctx context.Context, command RestoreCommand, reader io.Reader) (RestoreReport, error) {
	var report RestoreReport
	err := adapter.withWrite(ctx, func(tx *sql.Tx) error {
		current, err := adapter.loadState(ctx, tx, true)
		if err != nil {
			return err
		}
		next, value, err := restoreArchive(adapter.catalog, current, command, reader, adapter.clock().UTC())
		if err != nil {
			return err
		}
		report = value
		if !command.DryRun {
			return adapter.persistState(ctx, tx, current, next)
		}
		return nil
	})
	return report, err
}

func (adapter *PostgresAdapter) RebuildProjection(ctx context.Context, command RebuildCommand) (RebuildReport, error) {
	var report RebuildReport
	err := adapter.withWrite(ctx, func(tx *sql.Tx) error {
		state, err := adapter.loadState(ctx, tx, true)
		if err != nil {
			return err
		}
		report, err = rebuildState(adapter.catalog, &state, command)
		return err
	})
	return report, err
}

func (adapter *PostgresAdapter) withRead(ctx context.Context, run func(*sql.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if adapter.tx != nil {
		return run(adapter.tx)
	}
	tx, err := adapter.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return unavailable("begin read", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := run(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return unavailable("commit read", err)
	}
	return nil
}

func (adapter *PostgresAdapter) withWrite(ctx context.Context, run func(*sql.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if adapter.tx != nil {
		return run(adapter.tx)
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin write", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := run(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return unavailable("commit write", err)
	}
	return nil
}

func (adapter *PostgresAdapter) table(suffix string) string {
	return adapter.prefix + suffix
}

func unavailable(operation string, cause error) error {
	return &Error{Kind: ErrorUnavailable, Field: "postgres", Message: operation + " failed", Cause: cause}
}
