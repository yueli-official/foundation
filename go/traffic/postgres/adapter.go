package postgres

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/yueli-official/foundation/go/traffic"
)

type Options struct {
	DB          *sql.DB
	InstanceKey string
	Clock       func() time.Time
	Secret      []byte
	// InitialBaselines are imported in the same transaction that creates the
	// instance. They are ignored when the instance already exists.
	InitialBaselines []traffic.BaselineImport
}

type Adapter struct {
	db          *sql.DB
	instanceKey string
	catalog     *traffic.Catalog
	clock       func() time.Time
	secret      [32]byte
	created     bool
}

var _ traffic.Module = (*Adapter)(nil)

func New(ctx context.Context, catalog *traffic.Catalog, options Options) (*Adapter, error) {
	if catalog == nil {
		return nil, typedInvalid("catalog", "is required")
	}
	if options.DB == nil {
		return nil, typedInvalid("db", "is required")
	}
	instanceKey := strings.TrimSpace(options.InstanceKey)
	if instanceKey == "" {
		return nil, typedInvalid("instance_key", "is required")
	}
	if len(instanceKey) > 200 {
		return nil, typedInvalid("instance_key", "exceeds 200 bytes")
	}
	if strings.ContainsRune(instanceKey, '\x00') {
		return nil, typedInvalid("instance_key", "contains NUL")
	}
	if err := options.DB.PingContext(ctx); err != nil {
		return nil, unavailable("ping database", err)
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	var candidateSecret [32]byte
	switch {
	case len(options.Secret) == 0:
		if _, err := rand.Read(candidateSecret[:]); err != nil {
			return nil, unavailable("generate visitor secret", err)
		}
	case len(options.Secret) < len(candidateSecret):
		return nil, typedInvalid("secret", "must contain at least 32 bytes")
	default:
		candidateSecret = sha256.Sum256(options.Secret)
	}

	tx, err := options.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, unavailable("begin instance bootstrap", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
INSERT INTO traffic_instances (
    instance_key, schema_version, catalog_version, catalog_digest, time_zone, visitor_secret
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (instance_key) DO NOTHING`,
		instanceKey, CurrentSchemaVersion, catalog.Version(), catalog.Digest(), catalog.TimeZone(), candidateSecret[:])
	if err != nil {
		return nil, unavailable("bootstrap instance", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, unavailable("read bootstrap result", err)
	}
	var (
		schemaVersion  int64
		catalogVersion int64
		catalogDigest  string
		timeZone       string
		storedSecret   []byte
	)
	if err := tx.QueryRowContext(ctx, `
SELECT schema_version, catalog_version, catalog_digest, time_zone, visitor_secret
FROM traffic_instances
WHERE instance_key = $1
FOR UPDATE`, instanceKey).Scan(
		&schemaVersion, &catalogVersion, &catalogDigest, &timeZone, &storedSecret,
	); err != nil {
		return nil, unavailable("load instance", err)
	}
	if schemaVersion != int64(CurrentSchemaVersion) {
		return nil, &traffic.Error{
			Kind: traffic.ErrorUnavailable, Field: "schema_version",
			Message: fmt.Sprintf("database has %d, module requires %d", schemaVersion, CurrentSchemaVersion),
		}
	}
	if catalogVersion != int64(catalog.Version()) || catalogDigest != catalog.Digest() || timeZone != catalog.TimeZone() {
		return nil, &traffic.Error{
			Kind: traffic.ErrorConflict, Field: "catalog",
			Message: "database catalog version, digest, or time zone does not match the compiled definition",
		}
	}
	if len(storedSecret) != len(candidateSecret) {
		return nil, &traffic.Error{
			Kind: traffic.ErrorUnavailable, Field: "visitor_secret",
			Message: "database visitor secret is invalid",
		}
	}
	adapter := &Adapter{
		db: options.DB, instanceKey: instanceKey, catalog: catalog, clock: clock,
		created: affected == 1,
	}
	copy(adapter.secret[:], storedSecret)
	if adapter.created {
		for index, command := range options.InitialBaselines {
			prepared, err := prepareBaseline(catalog, command)
			if err != nil {
				return nil, fmt.Errorf("traffic: initial baseline %d: %w", index, err)
			}
			if _, err := importBaselineTx(ctx, tx, instanceKey, clock().UTC(), prepared); err != nil {
				return nil, fmt.Errorf("traffic: initial baseline %d: %w", index, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, unavailable("commit instance bootstrap", err)
	}
	return adapter, nil
}

func (adapter *Adapter) InstanceWasCreated() bool {
	return adapter != nil && adapter.created
}

func (adapter *Adapter) TokenizeVisitor(_ context.Context, at time.Time, seed []byte) (traffic.VisitorToken, error) {
	return adapter.catalog.DeriveVisitorToken(adapter.secret[:], at, seed)
}

func typedInvalid(field, message string) error {
	return &traffic.Error{Kind: traffic.ErrorInvalidInput, Field: field, Message: message}
}

func typedConflict(field, message string) error {
	return &traffic.Error{Kind: traffic.ErrorConflict, Field: field, Message: message}
}

func unavailable(operation string, err error) error {
	return &traffic.Error{
		Kind: traffic.ErrorUnavailable, Field: "postgres",
		Message: operation + " failed", Cause: err,
	}
}

func sameFingerprint(stored []byte, expected [32]byte) bool {
	return len(stored) == len(expected) && bytes.Equal(stored, expected[:])
}

type scopeColumns struct {
	kind         traffic.ScopeKind
	resourceKind traffic.ResourceKind
	resourceID   string
}

func columnsForScope(scope traffic.Scope) scopeColumns {
	if scope.Kind == traffic.ScopeInstance {
		return scopeColumns{kind: traffic.ScopeInstance}
	}
	return scopeColumns{
		kind:         traffic.ScopeResource,
		resourceKind: scope.Resource.Kind,
		resourceID:   scope.Resource.ID,
	}
}

func columnsForResource(resource traffic.Resource) scopeColumns {
	return scopeColumns{
		kind:         traffic.ScopeResource,
		resourceKind: resource.Kind,
		resourceID:   resource.ID,
	}
}

func instanceColumns() scopeColumns {
	return scopeColumns{kind: traffic.ScopeInstance}
}

func readTotals(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, instanceKey string, scope scopeColumns) (traffic.Totals, error) {
	var totals traffic.Totals
	err := queryer.QueryRowContext(ctx, `
SELECT COALESCE(views, 0), COALESCE(unique_visitor_days, 0)
FROM traffic_totals
WHERE instance_key = $1 AND scope_kind = $2 AND resource_kind = $3 AND resource_id = $4`,
		instanceKey, scope.kind, scope.resourceKind, scope.resourceID,
	).Scan(&totals.Views, &totals.UniqueVisitorDays)
	if err == sql.ErrNoRows {
		return traffic.Totals{}, nil
	}
	if err != nil {
		return traffic.Totals{}, err
	}
	return totals, nil
}
