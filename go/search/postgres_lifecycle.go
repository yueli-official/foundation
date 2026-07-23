package search

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (adapter *Postgres) verifyAnalyzer(ctx context.Context, key AnalyzerKey, binding string) error {
	var resolved sql.NullString
	if err := adapter.db.QueryRowContext(ctx, `SELECT $1::regconfig::text`, binding).Scan(&resolved); err != nil || !resolved.Valid {
		return &Error{Kind: ErrorUnsupportedCapability, Field: "analyzer." + string(key), Message: "PostgreSQL text search configuration is unavailable", Err: err}
	}
	definition := adapter.catalog.analyzers[key]
	for _, capability := range definition.Required {
		switch capability {
		case CapabilityFullText, CapabilityPhrase:
		case CapabilityChineseSegmentation:
			if definition.ProbeText == "" || len(definition.ProbeLexemes) == 0 {
				return &Error{Kind: ErrorInvalidDefinition, Field: "analyzer." + string(key), Message: "Chinese segmentation requires a lexeme probe"}
			}
		default:
			return &Error{Kind: ErrorUnsupportedCapability, Field: "analyzer." + string(key), Message: "requires an unsupported capability"}
		}
	}
	if definition.ProbeText != "" && len(definition.ProbeLexemes) > 0 {
		var vector string
		if err := adapter.db.QueryRowContext(ctx, `SELECT to_tsvector($1::regconfig,$2)::text`, binding, definition.ProbeText).Scan(&vector); err != nil {
			return &Error{Kind: ErrorUnsupportedCapability, Field: "analyzer." + string(key), Message: "probe failed", Err: err}
		}
		for _, lexeme := range definition.ProbeLexemes {
			if !strings.Contains(vector, "'"+lexeme+"'") {
				return &Error{Kind: ErrorUnsupportedCapability, Field: "analyzer." + string(key), Message: "probe lexeme is missing"}
			}
		}
	}
	return nil
}

func (adapter *Postgres) reconcile(ctx context.Context) error {
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin instance reconciliation", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := adapter.now().UTC()
	generation := GenerationID(randomID())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO search_instances(
			instance_key,schema_version,consumer,definition_version,definition_digest,
			active_generation,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$7)
		ON CONFLICT (instance_key) DO NOTHING
	`, adapter.instanceKey, PostgresSchemaVersion, adapter.catalog.definition.Consumer,
		adapter.catalog.definition.Version, adapter.catalog.digest, generation, now); err != nil {
		return unavailable("initialize search instance", err)
	}
	var schemaVersion int
	var consumer, digest string
	var version uint64
	var active GenerationID
	if err := tx.QueryRowContext(ctx, `
		SELECT schema_version,consumer,definition_version,definition_digest,active_generation
		FROM search_instances WHERE instance_key=$1 FOR UPDATE
	`, adapter.instanceKey).Scan(&schemaVersion, &consumer, &version, &digest, &active); err != nil {
		return unavailable("read search instance", err)
	}
	if schemaVersion != PostgresSchemaVersion || consumer != adapter.catalog.definition.Consumer ||
		version > adapter.catalog.definition.Version {
		return &Error{Kind: ErrorInvalidDefinition, Field: "instance", Message: "stored metadata is incompatible"}
	}
	if version != adapter.catalog.definition.Version || digest != adapter.catalog.digest {
		return &Error{Kind: ErrorInvalidDefinition, Field: "instance", Message: "definition changed without an explicit rebuild migration"}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO search_generations(
			instance_key,id,request_id,phase,definition_digest,started_at,updated_at
		) VALUES ($1,$2,$3,'active',$4,$5,$5)
		ON CONFLICT (instance_key,id) DO NOTHING
	`, adapter.instanceKey, active, "initial-"+string(active), adapter.catalog.digest, now); err != nil {
		return unavailable("initialize search generation", err)
	}
	if err := tx.Commit(); err != nil {
		return unavailable("commit instance reconciliation", err)
	}
	return nil
}

func (adapter *Postgres) lockInstance(ctx context.Context, tx *sql.Tx) (GenerationID, GenerationID, error) {
	var active GenerationID
	var building sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT active_generation,building_generation FROM search_instances
		WHERE instance_key=$1 FOR UPDATE
	`, adapter.instanceKey).Scan(&active, &building); err != nil {
		return "", "", unavailable("lock search instance", err)
	}
	return active, GenerationID(building.String), nil
}

func (adapter *Postgres) readReceipt(
	ctx context.Context, tx *sql.Tx, id BatchID,
) (ApplyResult, string, bool, error) {
	var result ApplyResult
	var fingerprint string
	err := tx.QueryRowContext(ctx, `
		SELECT fingerprint,applied,replays,stale FROM search_batch_receipts
		WHERE instance_key=$1 AND batch_id=$2
	`, adapter.instanceKey, id).Scan(&fingerprint, &result.Applied, &result.Replays, &result.Stale)
	if err == sql.ErrNoRows {
		return ApplyResult{}, "", false, nil
	}
	if err != nil {
		return ApplyResult{}, "", false, unavailable("read batch receipt", err)
	}
	return result, fingerprint, true, nil
}

func (adapter *Postgres) activeGeneration(ctx context.Context) (GenerationID, error) {
	var generation GenerationID
	if err := adapter.db.QueryRowContext(ctx, `
		SELECT active_generation FROM search_instances WHERE instance_key=$1
	`, adapter.instanceKey).Scan(&generation); err != nil {
		return "", unavailable("read active generation", err)
	}
	return generation, nil
}

func (adapter *Postgres) generationExists(ctx context.Context, id GenerationID) (bool, error) {
	var exists bool
	if err := adapter.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM search_generations WHERE instance_key=$1 AND id=$2)
	`, adapter.instanceKey, id).Scan(&exists); err != nil {
		return false, unavailable("check generation", err)
	}
	return exists, nil
}

func (adapter *Postgres) Start(ctx context.Context, request StartRebuild) (RebuildState, error) {
	if !stableName.MatchString(request.RequestID) {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "request_id", Message: "is invalid"}
	}
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return RebuildState{}, unavailable("begin rebuild", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, building, err := adapter.lockInstance(ctx, tx)
	if err != nil {
		return RebuildState{}, err
	}
	var existing GenerationID
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM search_generations WHERE instance_key=$1 AND request_id=$2
	`, adapter.instanceKey, request.RequestID).Scan(&existing)
	if err == nil {
		state, readErr := adapter.statusTx(ctx, tx, existing)
		if readErr != nil {
			return RebuildState{}, readErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return RebuildState{}, unavailable("finish rebuild lookup", commitErr)
		}
		return state, nil
	}
	if err != sql.ErrNoRows {
		return RebuildState{}, unavailable("lookup rebuild request", err)
	}
	if building != "" {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "another rebuild is active"}
	}
	id, now := GenerationID(randomID()), adapter.now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO search_generations(
			instance_key,id,request_id,phase,definition_digest,started_at,updated_at
		) VALUES ($1,$2,$3,'building',$4,$5,$5);
	`, adapter.instanceKey, id, request.RequestID, adapter.catalog.digest, now); err != nil {
		return RebuildState{}, unavailable("create rebuild generation", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE search_instances SET building_generation=$2,updated_at=$3 WHERE instance_key=$1
	`, adapter.instanceKey, id, now); err != nil {
		return RebuildState{}, unavailable("attach rebuild generation", err)
	}
	if err := tx.Commit(); err != nil {
		return RebuildState{}, unavailable("commit rebuild start", err)
	}
	return RebuildState{Generation: id, Phase: RebuildBuilding, StartedAt: now, UpdatedAt: now}, nil
}

func (adapter *Postgres) Stage(ctx context.Context, request RebuildBatch) (RebuildState, error) {
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return RebuildState{}, unavailable("begin rebuild stage", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, building, err := adapter.lockInstance(ctx, tx)
	if err != nil {
		return RebuildState{}, err
	}
	if building != request.Generation {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "is not building"}
	}
	if _, err := adapter.applyTx(ctx, tx, request.Batch, &request.Generation); err != nil {
		return RebuildState{}, err
	}
	now := adapter.now().UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE search_generations SET checkpoint=$3,
			document_count=(SELECT COUNT(*) FROM search_documents
				WHERE instance_key=$1 AND generation_id=$2 AND NOT deleted),
			updated_at=$4
		WHERE instance_key=$1 AND id=$2 AND phase='building'
	`, adapter.instanceKey, request.Generation, request.Checkpoint, now); err != nil {
		return RebuildState{}, unavailable("checkpoint rebuild", err)
	}
	state, err := adapter.statusTx(ctx, tx, request.Generation)
	if err != nil {
		return RebuildState{}, err
	}
	if err := tx.Commit(); err != nil {
		return RebuildState{}, unavailable("commit rebuild stage", err)
	}
	return state, nil
}

func (adapter *Postgres) Finish(ctx context.Context, request FinishRebuild) (RebuildState, error) {
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return RebuildState{}, unavailable("begin rebuild finish", err)
	}
	defer func() { _ = tx.Rollback() }()
	active, building, err := adapter.lockInstance(ctx, tx)
	if err != nil {
		return RebuildState{}, err
	}
	if building != request.Generation {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "is not building"}
	}
	state, err := adapter.statusTx(ctx, tx, request.Generation)
	if err != nil {
		return RebuildState{}, err
	}
	if state.Checkpoint != request.FinalCheckpoint || state.Documents != request.ExpectedDocuments {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "is incomplete"}
	}
	now := adapter.now().UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE search_generations SET phase='abandoned',updated_at=$3 WHERE instance_key=$1 AND id=$2
	`, adapter.instanceKey, active, now); err != nil {
		return RebuildState{}, unavailable("retire active generation", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE search_generations SET phase='active',updated_at=$3 WHERE instance_key=$1 AND id=$2
	`, adapter.instanceKey, request.Generation, now); err != nil {
		return RebuildState{}, unavailable("activate rebuild generation", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE search_instances SET active_generation=$2,building_generation=NULL,updated_at=$3 WHERE instance_key=$1
	`, adapter.instanceKey, request.Generation, now); err != nil {
		return RebuildState{}, unavailable("activate rebuild", err)
	}
	state.Phase, state.UpdatedAt = RebuildActive, now
	if err := tx.Commit(); err != nil {
		return RebuildState{}, unavailable("commit rebuild finish", err)
	}
	return state, nil
}

func (adapter *Postgres) Status(ctx context.Context, id GenerationID) (RebuildState, error) {
	return adapter.statusQuery(ctx, adapter.db, id)
}

func (adapter *Postgres) statusTx(ctx context.Context, tx *sql.Tx, id GenerationID) (RebuildState, error) {
	return adapter.statusQuery(ctx, tx, id)
}

type rowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (adapter *Postgres) statusQuery(ctx context.Context, queryer rowQueryer, id GenerationID) (RebuildState, error) {
	var state RebuildState
	err := queryer.QueryRowContext(ctx, `
		SELECT id,phase,checkpoint,document_count,started_at,updated_at
		FROM search_generations WHERE instance_key=$1 AND id=$2
	`, adapter.instanceKey, id).Scan(
		&state.Generation, &state.Phase, &state.Checkpoint, &state.Documents, &state.StartedAt, &state.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return RebuildState{}, &Error{Kind: ErrorGenerationGone, Field: "generation", Message: "does not exist"}
	}
	if err != nil {
		return RebuildState{}, unavailable("read rebuild state", err)
	}
	state.StartedAt, state.UpdatedAt = state.StartedAt.UTC(), state.UpdatedAt.UTC()
	return state, nil
}

func (adapter *Postgres) Abandon(ctx context.Context, id GenerationID) error {
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin rebuild abandon", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, building, err := adapter.lockInstance(ctx, tx)
	if err != nil {
		return err
	}
	if building != id {
		return &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "is not building"}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE search_instances SET building_generation=NULL,updated_at=$2 WHERE instance_key=$1
	`, adapter.instanceKey, adapter.now().UTC()); err != nil {
		return unavailable("detach rebuild generation", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM search_generations WHERE instance_key=$1 AND id=$2 AND phase='building'
	`, adapter.instanceKey, id); err != nil {
		return unavailable("abandon rebuild", err)
	}
	if err := tx.Commit(); err != nil {
		return unavailable("commit rebuild abandon", err)
	}
	return nil
}

func (adapter *Postgres) Prune(ctx context.Context, before time.Time) (int64, error) {
	result, err := adapter.db.ExecContext(ctx, `
		DELETE FROM search_generations
		WHERE instance_key=$1 AND phase='abandoned' AND updated_at < $2
		  AND id <> (SELECT active_generation FROM search_instances WHERE instance_key=$1)
		  AND id <> COALESCE((SELECT building_generation FROM search_instances WHERE instance_key=$1),'')
	`, adapter.instanceKey, before.UTC())
	if err != nil {
		return 0, unavailable("prune generations", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, unavailable("read pruned generation count", err)
	}
	return removed, nil
}

func firstAnalyzer(catalog *Catalog) AnalyzerKey {
	for key := range catalog.analyzers {
		return key
	}
	return ""
}

func unavailable(operation string, err error) error {
	return &Error{Kind: ErrorUnavailable, Field: "postgres", Message: fmt.Sprintf("%s failed", operation), Err: err}
}

var _ Module = (*Postgres)(nil)
var _ Projector = (*postgresProjector)(nil)
