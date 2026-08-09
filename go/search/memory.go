package search

import (
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/yueli-official/foundation/go/identifier"
)

type memoryRecord struct {
	Key      DocumentKey
	Revision ProjectionRevision
	Digest   string
	Document SourceDocument
	Filters  map[string][]string
	Deleted  bool
}

type memoryGeneration struct {
	State     RebuildState
	Documents map[DocumentKey]memoryRecord
}

type memoryReceipt struct {
	Fingerprint string
	Result      ApplyResult
}

type memoryRanked struct {
	record memoryRecord
	score  float32
}

type Memory struct {
	mu          sync.RWMutex
	catalog     *Catalog
	active      GenerationID
	building    GenerationID
	generations map[GenerationID]*memoryGeneration
	receipts    map[BatchID]memoryReceipt
	requests    map[string]GenerationID
	now         func() time.Time
}

func NewMemory(catalog *Catalog) *Memory {
	now := time.Now().UTC()
	generation := GenerationID("initial")
	return &Memory{
		catalog: catalog, active: generation,
		generations: map[GenerationID]*memoryGeneration{
			generation: {State: RebuildState{Generation: generation, Phase: RebuildActive, StartedAt: now, UpdatedAt: now}, Documents: map[DocumentKey]memoryRecord{}},
		},
		receipts: map[BatchID]memoryReceipt{}, requests: map[string]GenerationID{}, now: func() time.Time { return time.Now().UTC() },
	}
}

func (memory *Memory) Apply(_ context.Context, batch Batch) (ApplyResult, error) {
	prepared, err := memory.catalog.prepareBatch(batch)
	if err != nil {
		return ApplyResult{}, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if receipt, exists := memory.receipts[batch.ID]; exists {
		if receipt.Fingerprint != prepared.Fingerprint {
			return ApplyResult{}, &Error{Kind: ErrorIdempotencyConflict, Field: "batch.id", Message: "was already used for different changes"}
		}
		return receipt.Result, nil
	}
	targets := []GenerationID{memory.active}
	if memory.building != "" {
		targets = append(targets, memory.building)
	}
	result, err := memory.applyPrepared(prepared, targets)
	if err != nil {
		return ApplyResult{}, err
	}
	memory.receipts[batch.ID] = memoryReceipt{Fingerprint: prepared.Fingerprint, Result: result}
	return result, nil
}

func (memory *Memory) applyPrepared(batch preparedBatch, targets []GenerationID) (ApplyResult, error) {
	originals := make(map[GenerationID]map[DocumentKey]memoryRecord, len(targets))
	states := make(map[GenerationID]RebuildState, len(targets))
	for _, target := range targets {
		generation := memory.generations[target]
		states[target] = generation.State
		originals[target] = generation.Documents
		generation.Documents = cloneMemoryDocuments(generation.Documents)
	}
	result := ApplyResult{}
	for _, change := range batch.Changes {
		status := 0
		for _, generationID := range targets {
			generation := memory.generations[generationID]
			current, exists := generation.Documents[change.Key]
			switch {
			case exists && current.Revision > change.Revision:
				status = max(status, 1)
				continue
			case exists && current.Revision == change.Revision && current.Digest != change.Digest:
				for _, target := range targets {
					memory.generations[target].Documents = originals[target]
					memory.generations[target].State = states[target]
				}
				return ApplyResult{}, &Error{Kind: ErrorRevisionConflict, Field: "revision", Message: "already has different content"}
			case exists && current.Revision == change.Revision:
				status = max(status, 2)
				continue
			}
			record := memoryRecord{
				Key: change.Key, Revision: change.Revision, Digest: change.Digest,
				Document: change.Document, Filters: cloneFilterMap(change.Filters), Deleted: change.Kind == changeRemove,
			}
			generation.Documents[change.Key] = record
			generation.State.UpdatedAt = memory.now()
			status = 3
		}
		switch status {
		case 1:
			result.Stale++
		case 2:
			result.Replays++
		case 3:
			result.Applied++
		}
	}
	return result, nil
}

func cloneMemoryDocuments(input map[DocumentKey]memoryRecord) map[DocumentKey]memoryRecord {
	out := make(map[DocumentKey]memoryRecord, len(input))
	for key, value := range input {
		value.Filters = cloneFilterMap(value.Filters)
		value.Document.Keywords = slices.Clone(value.Document.Keywords)
		out[key] = value
	}
	return out
}

func (memory *Memory) Search(_ context.Context, query Query) (Page, error) {
	prepared, err := memory.catalog.prepareQuery(query)
	if err != nil {
		return Page{}, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	generationID := memory.active
	var after cursorPayload
	if prepared.Cursor != "" {
		after, err = decodeCursor(memory.catalog.digest, prepared.Cursor)
		cursorAge := memory.now().Sub(time.Unix(after.IssuedAt, 0))
		if err != nil || after.Query != prepared.Digest || after.Sort != prepared.Sort ||
			cursorAge < 0 || cursorAge > memory.catalog.definition.CursorTTL {
			return Page{}, invalidCursor()
		}
		generationID = after.Generation
	}
	generation, exists := memory.generations[generationID]
	if !exists {
		return Page{}, &Error{Kind: ErrorGenerationGone, Field: "cursor", Message: "references a retired generation"}
	}
	page := Page{Plan: PlanSummary{
		DefinitionDigest: memory.catalog.digest, Engine: "memory", Generation: generationID,
		QueryDigest: prepared.Digest, Sort: prepared.Sort,
	}}
	if prepared.Text == "" {
		return page, nil
	}
	var matches []memoryRanked
	for _, record := range generation.Documents {
		if record.Deleted || record.Document.Analyzer != prepared.Analyzer || !matchesFilters(record.Filters, prepared.Filters) {
			continue
		}
		score, matched := memoryScore(record.Document, prepared.Terms)
		if matched {
			matches = append(matches, memoryRanked{record: record, score: score})
		}
	}
	slices.SortFunc(matches, func(a, b memoryRanked) int {
		return compareRanked(a.score, a.record, b.score, b.record, prepared.Sort)
	})
	page.Total = uint64(len(matches))
	page.Facets = memoryFacets(matches, prepared.Facets)
	start := 0
	if prepared.Cursor != "" {
		for start < len(matches) && !rankedAfter(matches[start].score, matches[start].record, after, prepared.Sort) {
			start++
		}
	}
	end := min(start+prepared.Size, len(matches))
	for _, item := range matches[start:end] {
		page.Hits = append(page.Hits, Hit{
			Key: item.record.Key, Revision: item.record.Revision, Visibility: item.record.Document.Visibility,
			Score: item.score, Highlights: buildHighlights(item.record.Document, prepared.Terms, prepared.Highlights, memory.catalog.definition.Limits.MaxHighlightBytes),
		})
	}
	if end < len(matches) && end > start {
		last := matches[end-1]
		page.NextCursor = encodeCursor(memory.catalog.digest, cursorPayload{
			Generation: generationID, Query: prepared.Digest, Sort: prepared.Sort,
			ScoreBits: math.Float32bits(last.score), SortAt: last.record.Document.SortAt.UnixNano(),
			Kind: last.record.Key.Kind, ID: last.record.Key.ID, IssuedAt: memory.now().Unix(),
		})
	}
	return page, nil
}

func (memory *Memory) Start(_ context.Context, request StartRebuild) (RebuildState, error) {
	if !stableName.MatchString(request.RequestID) {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "request_id", Message: "is invalid"}
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if id, exists := memory.requests[request.RequestID]; exists {
		return memory.generations[id].State, nil
	}
	if memory.building != "" {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "another rebuild is active"}
	}
	id := GenerationID(randomID())
	now := memory.now()
	state := RebuildState{Generation: id, Phase: RebuildBuilding, StartedAt: now, UpdatedAt: now}
	memory.generations[id] = &memoryGeneration{State: state, Documents: map[DocumentKey]memoryRecord{}}
	memory.requests[request.RequestID], memory.building = id, id
	return state, nil
}

func (memory *Memory) Stage(_ context.Context, request RebuildBatch) (RebuildState, error) {
	prepared, err := memory.catalog.prepareBatch(request.Batch)
	if err != nil {
		return RebuildState{}, err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if request.Generation != memory.building {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "is not building"}
	}
	if receipt, exists := memory.receipts[request.Batch.ID]; exists {
		if receipt.Fingerprint != prepared.Fingerprint {
			return RebuildState{}, &Error{Kind: ErrorIdempotencyConflict, Field: "batch.id", Message: "was already used"}
		}
	} else {
		result, applyErr := memory.applyPrepared(prepared, []GenerationID{request.Generation})
		if applyErr != nil {
			return RebuildState{}, applyErr
		}
		memory.receipts[request.Batch.ID] = memoryReceipt{Fingerprint: prepared.Fingerprint, Result: result}
	}
	generation := memory.generations[request.Generation]
	generation.State.Checkpoint = request.Checkpoint
	generation.State.Documents = countLive(generation.Documents)
	generation.State.UpdatedAt = memory.now()
	return generation.State, nil
}

func (memory *Memory) Finish(_ context.Context, request FinishRebuild) (RebuildState, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if request.Generation != memory.building {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "is not building"}
	}
	generation := memory.generations[request.Generation]
	count := countLive(generation.Documents)
	if count != request.ExpectedDocuments || generation.State.Checkpoint != request.FinalCheckpoint {
		return RebuildState{}, &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "is incomplete"}
	}
	now := memory.now()
	generation.State.Phase, generation.State.Documents, generation.State.UpdatedAt = RebuildActive, count, now
	if old := memory.generations[memory.active]; old != nil {
		old.State.Phase, old.State.UpdatedAt = RebuildAbandoned, now
	}
	memory.active, memory.building = request.Generation, ""
	return generation.State, nil
}

func (memory *Memory) Status(_ context.Context, id GenerationID) (RebuildState, error) {
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	generation, exists := memory.generations[id]
	if !exists {
		return RebuildState{}, &Error{Kind: ErrorGenerationGone, Field: "generation", Message: "does not exist"}
	}
	return generation.State, nil
}

func (memory *Memory) Abandon(_ context.Context, id GenerationID) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if id != memory.building {
		return &Error{Kind: ErrorRebuildConflict, Field: "generation", Message: "is not building"}
	}
	delete(memory.generations, id)
	for request, generation := range memory.requests {
		if generation == id {
			delete(memory.requests, request)
		}
	}
	memory.building = ""
	return nil
}

func (memory *Memory) Prune(_ context.Context, before time.Time) (int64, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	var removed int64
	for id, generation := range memory.generations {
		if id != memory.active && id != memory.building && generation.State.Phase == RebuildAbandoned &&
			generation.State.UpdatedAt.Before(before) {
			delete(memory.generations, id)
			removed++
		}
	}
	return removed, nil
}

func memoryScore(document SourceDocument, terms []string) (float32, bool) {
	fields := []struct {
		value  string
		weight float32
	}{{document.Title, 8}, {strings.Join(document.Keywords, " "), 4}, {document.Summary, 2}, {document.Body, 1}}
	var score float32
	for _, term := range terms {
		found := false
		for _, field := range fields {
			count := strings.Count(strings.ToLower(field.value), term)
			if count > 0 {
				found = true
				score += float32(count) * field.weight
			}
		}
		if !found {
			return 0, false
		}
	}
	return score, score > 0
}

func compareRanked(as float32, a memoryRecord, bs float32, b memoryRecord, sort SortKind) int {
	if sort == SortRelevance && as != bs {
		if as > bs {
			return -1
		}
		return 1
	}
	if !a.Document.SortAt.Equal(b.Document.SortAt) {
		if a.Document.SortAt.After(b.Document.SortAt) {
			return -1
		}
		return 1
	}
	if a.Key.Kind != b.Key.Kind {
		if a.Key.Kind < b.Key.Kind {
			return -1
		}
		return 1
	}
	if a.Key.ID < b.Key.ID {
		return -1
	}
	if a.Key.ID > b.Key.ID {
		return 1
	}
	return 0
}

func rankedAfter(score float32, record memoryRecord, cursor cursorPayload, sort SortKind) bool {
	cursorRecord := memoryRecord{Key: DocumentKey{Kind: cursor.Kind, ID: cursor.ID}, Document: SourceDocument{SortAt: time.Unix(0, cursor.SortAt).UTC()}}
	return compareRanked(score, record, cursorScore(cursor), cursorRecord, sort) > 0
}

func matchesFilters(document map[string][]string, requested map[string]preparedFilter) bool {
	for name, filter := range requested {
		have := document[name]
		if filter.All {
			for _, value := range filter.Values {
				if !slices.Contains(have, value) {
					return false
				}
			}
		} else {
			found := false
			for _, value := range filter.Values {
				if slices.Contains(have, value) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func memoryFacets(matches []memoryRanked, requests []FacetRequest) []FacetResult {
	out := make([]FacetResult, 0, len(requests))
	for _, request := range requests {
		counts := map[string]uint64{}
		for _, match := range matches {
			for _, value := range match.record.Filters[string(request.Name)] {
				counts[value]++
			}
		}
		buckets := make([]FacetBucket, 0, len(counts))
		for value, count := range counts {
			buckets = append(buckets, FacetBucket{Value: value, Count: count})
		}
		slices.SortFunc(buckets, func(a, b FacetBucket) int {
			if a.Count != b.Count {
				if a.Count > b.Count {
					return -1
				}
				return 1
			}
			return strings.Compare(a.Value, b.Value)
		})
		if len(buckets) > request.Limit {
			buckets = buckets[:request.Limit]
		}
		out = append(out, FacetResult{Name: request.Name, Buckets: buckets})
	}
	return out
}

func buildHighlights(document SourceDocument, terms []string, fields []TextField, maxBytes int) []Highlight {
	var out []Highlight
	for _, field := range fields {
		var value string
		switch field {
		case TextTitle:
			value = document.Title
		case TextSummary:
			value = document.Summary
		case TextBody:
			value = document.Body
		}
		if len(value) > maxBytes {
			value = value[:maxBytes]
			for !utf8.ValidString(value) && len(value) > 0 {
				value = value[:len(value)-1]
			}
		}
		lower := strings.ToLower(value)
		start, length := -1, 0
		for _, term := range terms {
			if index := strings.Index(lower, term); index >= 0 && (start < 0 || index < start) {
				start, length = index, len(term)
			}
		}
		if start < 0 || start+length > len(value) {
			continue
		}
		segments := []Segment{}
		if start > 0 {
			segments = append(segments, Segment{Text: value[:start]})
		}
		segments = append(segments, Segment{Text: value[start : start+length], Matched: true})
		if start+length < len(value) {
			segments = append(segments, Segment{Text: value[start+length:]})
		}
		out = append(out, Highlight{Field: field, Fragments: []Fragment{{Segments: segments}}})
	}
	return out
}

func cloneFilterMap(input map[string][]string) map[string][]string {
	out := make(map[string][]string, len(input))
	for key, values := range input {
		out[key] = slices.Clone(values)
	}
	return out
}

func countLive(documents map[DocumentKey]memoryRecord) uint64 {
	var count uint64
	for _, value := range documents {
		if !value.Deleted {
			count++
		}
	}
	return count
}

func randomID() string {
	return identifier.MustNew().String()
}
