package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"
)

type PostgresOptions struct {
	DB               *sql.DB
	InstanceKey      string
	AnalyzerBindings map[AnalyzerKey]string
	Now              func() time.Time
}

type Postgres struct {
	db          *sql.DB
	instanceKey string
	catalog     *Catalog
	bindings    map[AnalyzerKey]string
	now         func() time.Time
}

type postgresProjector struct {
	adapter *Postgres
	tx      *sql.Tx
}

func NewPostgres(ctx context.Context, catalog *Catalog, options PostgresOptions) (*Postgres, error) {
	if options.DB == nil || catalog == nil || !stableName.MatchString(options.InstanceKey) {
		return nil, &Error{Kind: ErrorInvalidDefinition, Field: "postgres", Message: "options are invalid"}
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	adapter := &Postgres{
		db: options.DB, instanceKey: options.InstanceKey, catalog: catalog,
		bindings: make(map[AnalyzerKey]string, len(options.AnalyzerBindings)), now: options.Now,
	}
	for key := range catalog.analyzers {
		binding := strings.TrimSpace(options.AnalyzerBindings[key])
		if binding == "" {
			return nil, &Error{Kind: ErrorUnsupportedCapability, Field: "analyzer." + string(key), Message: "has no PostgreSQL configuration binding"}
		}
		adapter.bindings[key] = binding
		if err := adapter.verifyAnalyzer(ctx, key, binding); err != nil {
			return nil, err
		}
	}
	if err := adapter.reconcile(ctx); err != nil {
		return nil, err
	}
	return adapter, nil
}

func (adapter *Postgres) Bind(tx *sql.Tx) (Projector, error) {
	if tx == nil {
		return nil, &Error{Kind: ErrorTransactionRequired, Field: "transaction", Message: "is required"}
	}
	return &postgresProjector{adapter: adapter, tx: tx}, nil
}

func (adapter *Postgres) Apply(ctx context.Context, batch Batch) (ApplyResult, error) {
	tx, err := adapter.db.BeginTx(ctx, nil)
	if err != nil {
		return ApplyResult{}, unavailable("begin projection transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := adapter.applyTx(ctx, tx, batch, nil)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ApplyResult{}, unavailable("commit projection transaction", err)
	}
	return result, nil
}

func (bound *postgresProjector) Apply(ctx context.Context, batch Batch) (ApplyResult, error) {
	return bound.adapter.applyTx(ctx, bound.tx, batch, nil)
}

func (adapter *Postgres) applyTx(ctx context.Context, tx *sql.Tx, batch Batch, only *GenerationID) (ApplyResult, error) {
	prepared, err := adapter.catalog.prepareBatch(batch)
	if err != nil {
		return ApplyResult{}, err
	}
	active, building, err := adapter.lockInstance(ctx, tx)
	if err != nil {
		return ApplyResult{}, err
	}
	if result, fingerprint, found, err := adapter.readReceipt(ctx, tx, batch.ID); err != nil {
		return ApplyResult{}, err
	} else if found {
		if fingerprint != prepared.Fingerprint {
			return ApplyResult{}, &Error{Kind: ErrorIdempotencyConflict, Field: "batch.id", Message: "was already used for different changes"}
		}
		return result, nil
	}
	targets := []GenerationID{active}
	if only != nil {
		targets = []GenerationID{*only}
	} else if building != "" {
		targets = append(targets, building)
	}
	type decision struct {
		change preparedChange
		target GenerationID
		write  bool
		status int
	}
	var decisions []decision
	changeStatus := make([]int, len(prepared.Changes))
	for index, change := range prepared.Changes {
		for _, target := range targets {
			revision, digest, found, readErr := adapter.currentProjection(ctx, tx, target, change.Key)
			if readErr != nil {
				return ApplyResult{}, readErr
			}
			item := decision{change: change, target: target, write: true, status: 3}
			switch {
			case found && revision > change.Revision:
				item.write, item.status = false, 1
			case found && revision == change.Revision && digest != change.Digest:
				return ApplyResult{}, &Error{Kind: ErrorRevisionConflict, Field: "revision", Message: "already has different content"}
			case found && revision == change.Revision:
				item.write, item.status = false, 2
			}
			changeStatus[index] = max(changeStatus[index], item.status)
			decisions = append(decisions, item)
		}
	}
	for _, item := range decisions {
		if item.write {
			if err := adapter.writeProjection(ctx, tx, item.target, item.change); err != nil {
				return ApplyResult{}, err
			}
		}
	}
	result := ApplyResult{}
	for _, status := range changeStatus {
		switch status {
		case 1:
			result.Stale++
		case 2:
			result.Replays++
		case 3:
			result.Applied++
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO search_batch_receipts(
			instance_key, batch_id, fingerprint, applied, replays, stale, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, adapter.instanceKey, batch.ID, prepared.Fingerprint, result.Applied, result.Replays, result.Stale, adapter.now().UTC()); err != nil {
		return ApplyResult{}, unavailable("store batch receipt", err)
	}
	return result, nil
}

func (adapter *Postgres) currentProjection(
	ctx context.Context, tx *sql.Tx, generation GenerationID, key DocumentKey,
) (ProjectionRevision, string, bool, error) {
	var revision ProjectionRevision
	var digest string
	err := tx.QueryRowContext(ctx, `
		SELECT revision, content_digest FROM search_documents
		WHERE instance_key=$1 AND generation_id=$2 AND document_kind=$3 AND document_id=$4
		FOR UPDATE
	`, adapter.instanceKey, generation, key.Kind, key.ID).Scan(&revision, &digest)
	if err == sql.ErrNoRows {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, unavailable("read projection", err)
	}
	return revision, digest, true, nil
}

func (adapter *Postgres) writeProjection(ctx context.Context, tx *sql.Tx, generation GenerationID, change preparedChange) error {
	document := change.Document
	config := adapter.bindings[document.Analyzer]
	deleted := change.Kind == changeRemove
	if deleted {
		document = SourceDocument{
			Key: change.Key, Revision: change.Revision, Analyzer: firstAnalyzer(adapter.catalog),
			SortAt: adapter.now().UTC(), Keywords: []string{},
		}
		config = adapter.bindings[document.Analyzer]
	}
	filterJSON, _ := json.Marshal(change.Filters)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO search_documents(
			instance_key,generation_id,document_kind,document_id,revision,content_digest,
			analyzer,title,summary,body,keywords,filters,sort_at,visibility_type,visibility_id,
			deleted,search_vector,updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,
			setweight(to_tsvector($17::regconfig,$8),'A') ||
			setweight(to_tsvector($17::regconfig,array_to_string($11::text[],' ')),'B') ||
			setweight(to_tsvector($17::regconfig,$9),'C') ||
			setweight(to_tsvector($17::regconfig,$10),'D'),$18
		)
		ON CONFLICT (instance_key,generation_id,document_kind,document_id) DO UPDATE SET
			revision=EXCLUDED.revision,content_digest=EXCLUDED.content_digest,analyzer=EXCLUDED.analyzer,
			title=EXCLUDED.title,summary=EXCLUDED.summary,body=EXCLUDED.body,keywords=EXCLUDED.keywords,
			filters=EXCLUDED.filters,sort_at=EXCLUDED.sort_at,visibility_type=EXCLUDED.visibility_type,
			visibility_id=EXCLUDED.visibility_id,deleted=EXCLUDED.deleted,
			search_vector=EXCLUDED.search_vector,updated_at=EXCLUDED.updated_at
	`, adapter.instanceKey, generation, change.Key.Kind, change.Key.ID, change.Revision, change.Digest,
		document.Analyzer, document.Title, document.Summary, document.Body, pq.Array(document.Keywords),
		filterJSON, document.SortAt, document.Visibility.ResourceType, document.Visibility.ResourceID,
		deleted, config, adapter.now().UTC())
	if err != nil {
		return unavailable("write projection", err)
	}
	return nil
}

func (adapter *Postgres) Search(ctx context.Context, query Query) (Page, error) {
	prepared, err := adapter.catalog.prepareQuery(query)
	if err != nil {
		return Page{}, err
	}
	generation, err := adapter.activeGeneration(ctx)
	if err != nil {
		return Page{}, err
	}
	var after cursorPayload
	if prepared.Cursor != "" {
		after, err = decodeCursor(adapter.catalog.digest, prepared.Cursor)
		cursorAge := adapter.now().Sub(time.Unix(after.IssuedAt, 0))
		if err != nil || after.Query != prepared.Digest || after.Sort != prepared.Sort ||
			cursorAge < 0 || cursorAge > adapter.catalog.definition.CursorTTL {
			return Page{}, invalidCursor()
		}
		generation = after.Generation
		if exists, checkErr := adapter.generationExists(ctx, generation); checkErr != nil {
			return Page{}, checkErr
		} else if !exists {
			return Page{}, &Error{Kind: ErrorGenerationGone, Field: "cursor", Message: "references a retired generation"}
		}
	}
	page := Page{Plan: PlanSummary{
		DefinitionDigest: adapter.catalog.digest, Engine: "postgres", Generation: generation,
		QueryDigest: prepared.Digest, Sort: prepared.Sort,
	}}
	if prepared.Text == "" {
		return page, nil
	}
	base, args := adapter.searchPredicate(generation, prepared)
	rank := adapter.rankExpression(prepared)
	var total uint64
	if err := adapter.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM search_documents d WHERE "+base, args...).Scan(&total); err != nil {
		return Page{}, unavailable("count search results", err)
	}
	page.Total = total
	for _, facet := range prepared.Facets {
		facetArgs := append(slices.Clone(args), string(facet.Name), facet.Limit)
		statement := `SELECT value, COUNT(*) AS count
			FROM search_documents d
			CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(d.filters -> $` + fmt.Sprint(len(args)+1) + `,'[]'::jsonb)) value
			WHERE ` + base + `
			GROUP BY value ORDER BY count DESC, value ASC LIMIT $` + fmt.Sprint(len(args)+2)
		rows, queryErr := adapter.db.QueryContext(ctx, statement, facetArgs...)
		if queryErr != nil {
			return Page{}, unavailable("query facet", queryErr)
		}
		result := FacetResult{Name: facet.Name}
		for rows.Next() {
			var bucket FacetBucket
			if scanErr := rows.Scan(&bucket.Value, &bucket.Count); scanErr != nil {
				_ = rows.Close()
				return Page{}, unavailable("scan facet", scanErr)
			}
			result.Buckets = append(result.Buckets, bucket)
		}
		if rowErr := rows.Err(); rowErr != nil {
			_ = rows.Close()
			return Page{}, unavailable("iterate facet", rowErr)
		}
		_ = rows.Close()
		page.Facets = append(page.Facets, result)
	}
	cte := "WITH ranked AS (SELECT d.*, " + rank + " AS rank FROM search_documents d WHERE " + base + ") SELECT " +
		"document_kind,document_id,revision,visibility_type,visibility_id,rank,sort_at,title,summary,body FROM ranked"
	rowArgs := slices.Clone(args)
	if prepared.Cursor != "" {
		if prepared.Sort == SortRelevance {
			rowArgs = append(rowArgs, cursorScore(after), time.Unix(0, after.SortAt).UTC(), after.Kind, after.ID)
			n := len(rowArgs)
			cte += fmt.Sprintf(` WHERE (rank < $%d OR (rank = $%d AND
				(sort_at < $%d OR (sort_at = $%d AND
				(document_kind > $%d OR (document_kind = $%d AND document_id > $%d))))))`,
				n-3, n-3, n-2, n-2, n-1, n-1, n)
		} else {
			rowArgs = append(rowArgs, time.Unix(0, after.SortAt).UTC(), after.Kind, after.ID)
			n := len(rowArgs)
			cte += fmt.Sprintf(` WHERE (sort_at < $%d OR (sort_at = $%d AND
				(document_kind > $%d OR (document_kind = $%d AND document_id > $%d))))`,
				n-2, n-2, n-1, n-1, n)
		}
	}
	if prepared.Sort == SortRelevance {
		cte += " ORDER BY rank DESC, sort_at DESC, document_kind ASC, document_id ASC"
	} else {
		cte += " ORDER BY sort_at DESC, document_kind ASC, document_id ASC"
	}
	rowArgs = append(rowArgs, prepared.Size+1)
	cte += fmt.Sprintf(" LIMIT $%d", len(rowArgs))
	rows, err := adapter.db.QueryContext(ctx, cte, rowArgs...)
	if err != nil {
		return Page{}, unavailable("query search results", err)
	}
	defer rows.Close()
	type rowValue struct {
		hit                  Hit
		score                float32
		sortAt               time.Time
		title, summary, body string
	}
	var values []rowValue
	for rows.Next() {
		var value rowValue
		if err := rows.Scan(&value.hit.Key.Kind, &value.hit.Key.ID, &value.hit.Revision,
			&value.hit.Visibility.ResourceType, &value.hit.Visibility.ResourceID, &value.score,
			&value.sortAt, &value.title, &value.summary, &value.body); err != nil {
			return Page{}, unavailable("scan search result", err)
		}
		value.hit.Score = value.score
		value.hit.Highlights = buildHighlights(SourceDocument{
			Title: value.title, Summary: value.summary, Body: value.body,
		}, prepared.Terms, prepared.Highlights, adapter.catalog.definition.Limits.MaxHighlightBytes)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return Page{}, unavailable("iterate search result", err)
	}
	if len(values) > prepared.Size {
		last := values[prepared.Size-1]
		page.NextCursor = encodeCursor(adapter.catalog.digest, cursorPayload{
			Generation: generation, Query: prepared.Digest, Sort: prepared.Sort,
			ScoreBits: math.Float32bits(last.score), SortAt: last.sortAt.UnixNano(),
			Kind: last.hit.Key.Kind, ID: last.hit.Key.ID, IssuedAt: adapter.now().Unix(),
		})
		values = values[:prepared.Size]
	}
	for _, value := range values {
		page.Hits = append(page.Hits, value.hit)
	}
	return page, nil
}

func (adapter *Postgres) searchPredicate(generation GenerationID, query preparedQuery) (string, []any) {
	config := adapter.bindings[query.Analyzer]
	args := []any{adapter.instanceKey, generation, query.Analyzer, config, query.Text}
	parser := "websearch_to_tsquery"
	if adapter.catalog.analyzers[query.Analyzer].QueryMode == QueryPlain {
		parser = "plainto_tsquery"
	}
	where := fmt.Sprintf(
		"d.instance_key=$1 AND d.generation_id=$2 AND d.analyzer=$3 AND NOT d.deleted AND d.search_vector @@ %s($4::regconfig,$5)",
		parser,
	)
	names := make([]string, 0, len(query.Filters))
	for name := range query.Filters {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		filter := query.Filters[name]
		if filter.All {
			encoded, _ := json.Marshal(map[string][]string{name: filter.Values})
			args = append(args, encoded)
			where += fmt.Sprintf(" AND d.filters @> $%d::jsonb", len(args))
		} else {
			args = append(args, name, pq.Array(filter.Values))
			where += fmt.Sprintf(" AND (d.filters -> $%d) ?| $%d", len(args)-1, len(args))
		}
	}
	return where, args
}

func (adapter *Postgres) rankExpression(query preparedQuery) string {
	parser := "websearch_to_tsquery"
	if adapter.catalog.analyzers[query.Analyzer].QueryMode == QueryPlain {
		parser = "plainto_tsquery"
	}
	return fmt.Sprintf("ts_rank_cd(d.search_vector,%s($4::regconfig,$5))", parser)
}
