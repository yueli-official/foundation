package traffic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

type MemoryOptions struct {
	Clock  func() time.Time
	Secret []byte
}

type scopeKey struct {
	kind         ScopeKind
	resourceKind ResourceKind
	resourceID   string
}

type dailyKey struct {
	day   Day
	scope scopeKey
}

type visitorKey struct {
	daily dailyKey
	token VisitorToken
}

type memoryReceipt struct {
	prepared      PreparedObservation
	received      time.Time
	firstInstance bool
	firstResource bool
}

type baselineKey struct {
	source   string
	resource Resource
}

type memoryBaseline struct {
	views       int64
	fingerprint [32]byte
}

// Memory is the reference Adapter and a useful deterministic test
// implementation. It provides the same observable contract as postgres.Adapter.
type Memory struct {
	mu        sync.RWMutex
	catalog   *Catalog
	clock     func() time.Time
	secret    [32]byte
	receipts  map[EventID]memoryReceipt
	totals    map[scopeKey]Totals
	daily     map[dailyKey]Totals
	visitors  map[visitorKey]struct{}
	baselines map[baselineKey]memoryBaseline
}

var _ Module = (*Memory)(nil)

func NewMemory(catalog *Catalog, options MemoryOptions) (*Memory, error) {
	if catalog == nil {
		return nil, invalid("catalog", "is required")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	var secret [32]byte
	switch {
	case len(options.Secret) == 0:
		if _, err := rand.Read(secret[:]); err != nil {
			return nil, &Error{Kind: ErrorUnavailable, Field: "secret", Message: "cannot generate visitor secret", Cause: err}
		}
	case len(options.Secret) < len(secret):
		return nil, invalid("secret", "must contain at least %d bytes", len(secret))
	default:
		sum := sha256.Sum256(options.Secret)
		copy(secret[:], sum[:])
	}
	return &Memory{
		catalog: catalog, clock: clock, secret: secret,
		receipts:  make(map[EventID]memoryReceipt),
		totals:    make(map[scopeKey]Totals),
		daily:     make(map[dailyKey]Totals),
		visitors:  make(map[visitorKey]struct{}),
		baselines: make(map[baselineKey]memoryBaseline),
	}, nil
}

func (memory *Memory) TokenizeVisitor(_ context.Context, at time.Time, seed []byte) (VisitorToken, error) {
	return memory.catalog.DeriveVisitorToken(memory.secret[:], at, seed)
}

func (memory *Memory) Record(ctx context.Context, observation Observation) (RecordResult, error) {
	results, err := memory.RecordBatch(ctx, []Observation{observation})
	if err != nil {
		return RecordResult{}, err
	}
	return results[0], nil
}

func (memory *Memory) RecordBatch(ctx context.Context, observations []Observation) ([]RecordResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(observations) == 0 {
		return nil, invalid("observations", "must contain at least one item")
	}
	if len(observations) > memory.catalog.limits.MaxBatchSize {
		return nil, invalid("observations", "exceeds max batch size %d", memory.catalog.limits.MaxBatchSize)
	}
	now := memory.clock().UTC()
	prepared := make([]PreparedObservation, len(observations))
	for index, observation := range observations {
		value, err := memory.catalog.PrepareObservation(now, observation)
		if err != nil {
			return nil, prefixIndex(err, "observations", index)
		}
		prepared[index] = value
	}

	memory.mu.Lock()
	defer memory.mu.Unlock()
	seen := make(map[EventID][32]byte, len(prepared))
	for _, value := range prepared {
		if fingerprint, ok := seen[value.EventID]; ok && fingerprint != value.Fingerprint {
			return nil, conflict("event_id", "%q is reused with a different payload", value.EventID)
		}
		seen[value.EventID] = value.Fingerprint
		if receipt, ok := memory.receipts[value.EventID]; ok && receipt.prepared.Fingerprint != value.Fingerprint {
			return nil, conflict("event_id", "%q is reused with a different payload", value.EventID)
		}
	}

	results := make([]RecordResult, 0, len(prepared))
	for _, value := range prepared {
		if receipt, ok := memory.receipts[value.EventID]; ok {
			results = append(results, memory.resultLocked(receipt, true))
			continue
		}
		receipt := memoryReceipt{prepared: value, received: now}
		if value.Counted {
			instance := instanceScopeKey()
			resource := resourceScopeKey(value.Resource)
			memory.incrementViewLocked(value.Day, instance)
			memory.incrementViewLocked(value.Day, resource)
			if value.HasVisitor {
				receipt.firstInstance = memory.markVisitorLocked(value.Day, instance, value.VisitorToken)
				receipt.firstResource = memory.markVisitorLocked(value.Day, resource, value.VisitorToken)
			}
		}
		memory.receipts[value.EventID] = receipt
		results = append(results, memory.resultLocked(receipt, false))
	}
	return results, nil
}

func prefixIndex(err error, field string, index int) error {
	var typed *Error
	if !errors.As(err, &typed) {
		return err
	}
	copy := *typed
	if copy.Field == "" {
		copy.Field = fmt.Sprintf("%s[%d]", field, index)
	} else {
		copy.Field = fmt.Sprintf("%s[%d].%s", field, index, copy.Field)
	}
	return &copy
}

func (memory *Memory) incrementViewLocked(day Day, scope scopeKey) {
	total := memory.totals[scope]
	total.Views++
	memory.totals[scope] = total
	key := dailyKey{day: day, scope: scope}
	value := memory.daily[key]
	value.Views++
	memory.daily[key] = value
}

func (memory *Memory) markVisitorLocked(day Day, scope scopeKey, token VisitorToken) bool {
	key := visitorKey{daily: dailyKey{day: day, scope: scope}, token: token}
	if _, exists := memory.visitors[key]; exists {
		return false
	}
	memory.visitors[key] = struct{}{}
	total := memory.totals[scope]
	total.UniqueVisitorDays++
	memory.totals[scope] = total
	daily := memory.daily[key.daily]
	daily.UniqueVisitorDays++
	memory.daily[key.daily] = daily
	return true
}

func (memory *Memory) resultLocked(receipt memoryReceipt, replay bool) RecordResult {
	return RecordResult{
		EventID: receipt.prepared.EventID, Counted: receipt.prepared.Counted,
		Replay: replay, DropReason: receipt.prepared.DropReason,
		FirstInstanceVisitor: receipt.firstInstance,
		FirstResourceVisitor: receipt.firstResource,
		InstanceTotals:       memory.totals[instanceScopeKey()],
		ResourceTotals:       memory.totals[resourceScopeKey(receipt.prepared.Resource)],
	}
}

func (memory *Memory) Summary(ctx context.Context, query SummaryQuery) (Summary, error) {
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	scope, err := memory.catalog.NormalizeScope(query.Scope)
	if err != nil {
		return Summary{}, err
	}
	var normalizedRange *DateRange
	if query.Range != nil {
		value, err := memory.catalog.NormalizeRange(*query.Range)
		if err != nil {
			return Summary{}, err
		}
		normalizedRange = &value
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	key := keyFromScope(scope)
	var totals Totals
	if normalizedRange == nil {
		totals = memory.totals[key]
	} else {
		for day := normalizedRange.From; day != normalizedRange.To; day = memory.catalog.dayAdd(day, 1) {
			totals = addTotals(totals, memory.daily[dailyKey{day: day, scope: key}])
		}
	}
	return Summary{Scope: scope, Range: normalizedRange, Totals: totals}, nil
}

func (memory *Memory) Series(ctx context.Context, query SeriesQuery) ([]SeriesPoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scope, err := memory.catalog.NormalizeScope(query.Scope)
	if err != nil {
		return nil, err
	}
	dateRange, err := memory.catalog.NormalizeRange(query.Range)
	if err != nil {
		return nil, err
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	key := keyFromScope(scope)
	result := make([]SeriesPoint, 0)
	for day := dateRange.From; day != dateRange.To; day = memory.catalog.dayAdd(day, 1) {
		result = append(result, SeriesPoint{Day: day, Totals: memory.daily[dailyKey{day: day, scope: key}]})
	}
	return result, nil
}

func (memory *Memory) Top(ctx context.Context, query TopQuery) ([]TopEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kind := ResourceKind(strings.TrimSpace(string(query.ResourceKind)))
	if _, ok := memory.catalog.resourceKinds[kind]; !ok {
		return nil, invalid("resource_kind", "is not registered")
	}
	metric := query.Metric
	if metric == "" {
		metric = RankViews
	}
	if metric != RankViews && metric != RankUniqueVisitorDays {
		return nil, invalid("metric", "is unknown")
	}
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 1000 {
		return nil, invalid("limit", "must be between 1 and 1000")
	}
	var normalizedRange *DateRange
	if query.Range != nil {
		value, err := memory.catalog.NormalizeRange(*query.Range)
		if err != nil {
			return nil, err
		}
		normalizedRange = &value
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	values := make(map[scopeKey]Totals)
	if normalizedRange == nil {
		for key, totals := range memory.totals {
			if key.kind == ScopeResource && key.resourceKind == kind {
				values[key] = totals
			}
		}
	} else {
		for key, totals := range memory.daily {
			if key.scope.kind != ScopeResource || key.scope.resourceKind != kind {
				continue
			}
			if dayInRange(key.day, *normalizedRange, memory.catalog.location) {
				values[key.scope] = addTotals(values[key.scope], totals)
			}
		}
	}
	result := make([]TopEntry, 0, len(values))
	for key, totals := range values {
		result = append(result, TopEntry{
			Resource: Resource{Kind: key.resourceKind, ID: key.resourceID},
			Totals:   totals,
		})
	}
	slices.SortStableFunc(result, func(left, right TopEntry) int {
		leftValue, rightValue := left.Totals.Views, right.Totals.Views
		if metric == RankUniqueVisitorDays {
			leftValue, rightValue = left.Totals.UniqueVisitorDays, right.Totals.UniqueVisitorDays
		}
		if leftValue > rightValue {
			return -1
		}
		if leftValue < rightValue {
			return 1
		}
		return strings.Compare(left.Resource.ID, right.Resource.ID)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (memory *Memory) Totals(ctx context.Context, resources []Resource) ([]ResourceTotals, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(resources) > 1000 {
		return nil, invalid("resources", "exceeds 1000 items")
	}
	normalized := make([]Resource, len(resources))
	for index, resource := range resources {
		value, err := memory.catalog.NormalizeResource(resource)
		if err != nil {
			return nil, prefixIndex(err, "resources", index)
		}
		normalized[index] = value
	}
	memory.mu.RLock()
	defer memory.mu.RUnlock()
	result := make([]ResourceTotals, len(normalized))
	for index, resource := range normalized {
		result[index] = ResourceTotals{
			Resource: resource,
			Totals:   memory.totals[resourceScopeKey(resource)],
		}
	}
	return result, nil
}

func (memory *Memory) ImportBaseline(ctx context.Context, command BaselineImport) (ImportResult, error) {
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}
	source := strings.TrimSpace(command.Source)
	if source == "" {
		return ImportResult{}, invalid("source", "is required")
	}
	if len(source) > memory.catalog.limits.MaxBaselineSourceBytes {
		return ImportResult{}, invalid("source", "exceeds %d bytes", memory.catalog.limits.MaxBaselineSourceBytes)
	}
	if strings.ContainsRune(source, '\x00') {
		return ImportResult{}, invalid("source", "contains NUL")
	}
	resource, err := memory.catalog.NormalizeResource(command.Resource)
	if err != nil {
		return ImportResult{}, err
	}
	if command.Views < 0 {
		return ImportResult{}, invalid("views", "must not be negative")
	}
	fingerprint := baselineFingerprint(source, resource, command.Views)
	key := baselineKey{source: source, resource: resource}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if existing, ok := memory.baselines[key]; ok {
		if existing.fingerprint != fingerprint {
			return ImportResult{}, conflict("source", "%q is reused with a different baseline", source)
		}
		return ImportResult{
			Replay:         true,
			ResourceTotals: memory.totals[resourceScopeKey(resource)],
		}, nil
	}
	memory.baselines[key] = memoryBaseline{views: command.Views, fingerprint: fingerprint}
	for _, scope := range []scopeKey{instanceScopeKey(), resourceScopeKey(resource)} {
		total := memory.totals[scope]
		total.Views += command.Views
		memory.totals[scope] = total
	}
	return ImportResult{
		Applied:        true,
		ResourceTotals: memory.totals[resourceScopeKey(resource)],
	}, nil
}

func (memory *Memory) Prune(ctx context.Context, now time.Time) (PruneResult, error) {
	if err := ctx.Err(); err != nil {
		return PruneResult{}, err
	}
	if now.IsZero() {
		return PruneResult{}, invalid("now", "is required")
	}
	now = now.UTC()
	receiptCutoff := now.Add(-memory.catalog.limits.ReceiptRetention)
	markerCutoff := now.Add(-memory.catalog.limits.VisitorMarkerRetention)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	var result PruneResult
	for key, receipt := range memory.receipts {
		if receipt.received.Before(receiptCutoff) {
			delete(memory.receipts, key)
			result.ReceiptsRemoved++
		}
	}
	for key := range memory.visitors {
		if !memory.catalog.dayEnd(key.daily.day).After(markerCutoff) {
			delete(memory.visitors, key)
			result.VisitorMarkersRemoved++
		}
	}
	return result, nil
}

func (memory *Memory) ForgetResource(ctx context.Context, resource Resource) (ForgetResult, error) {
	if err := ctx.Err(); err != nil {
		return ForgetResult{}, err
	}
	resource, err := memory.catalog.NormalizeResource(resource)
	if err != nil {
		return ForgetResult{}, err
	}
	scope := resourceScopeKey(resource)
	memory.mu.Lock()
	defer memory.mu.Unlock()
	var result ForgetResult
	if _, exists := memory.totals[scope]; exists {
		delete(memory.totals, scope)
		result.TotalsRemoved++
	}
	for key := range memory.daily {
		if key.scope == scope {
			delete(memory.daily, key)
			result.DailyRowsRemoved++
		}
	}
	for key := range memory.visitors {
		if key.daily.scope == scope {
			delete(memory.visitors, key)
			result.VisitorMarkersRemoved++
		}
	}
	for key, receipt := range memory.receipts {
		if receipt.prepared.Resource == resource {
			delete(memory.receipts, key)
			result.ReceiptsRemoved++
		}
	}
	for key := range memory.baselines {
		if key.resource == resource {
			delete(memory.baselines, key)
			result.BaselinesRemoved++
		}
	}
	return result, nil
}

func instanceScopeKey() scopeKey {
	return scopeKey{kind: ScopeInstance}
}

func resourceScopeKey(resource Resource) scopeKey {
	return scopeKey{kind: ScopeResource, resourceKind: resource.Kind, resourceID: resource.ID}
}

func keyFromScope(scope Scope) scopeKey {
	if scope.Kind == ScopeInstance {
		return instanceScopeKey()
	}
	return resourceScopeKey(scope.Resource)
}

func addTotals(left, right Totals) Totals {
	return Totals{
		Views:             left.Views + right.Views,
		UniqueVisitorDays: left.UniqueVisitorDays + right.UniqueVisitorDays,
	}
}

func dayInRange(day Day, dateRange DateRange, location *time.Location) bool {
	value := day.at(location)
	return !value.Before(dateRange.From.at(location)) && value.Before(dateRange.To.at(location))
}

func baselineFingerprint(source string, resource Resource, views int64) [32]byte {
	value := fmt.Sprintf("%s\x00%s\x00%s\x00%d", source, resource.Kind, resource.ID, views)
	return sha256.Sum256([]byte(value))
}
