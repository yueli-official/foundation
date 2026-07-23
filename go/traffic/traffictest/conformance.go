// Package traffictest provides the public Adapter conformance suite.
package traffictest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/traffic"
)

type Clock struct {
	mu  sync.RWMutex
	now time.Time
}

func NewClock(now time.Time) *Clock {
	return &Clock{now: now}
}

func (clock *Clock) Now() time.Time {
	clock.mu.RLock()
	defer clock.mu.RUnlock()
	return clock.now
}

func (clock *Clock) Set(now time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = now
}

type Factory func(*testing.T, *traffic.Catalog, *Clock) traffic.Module

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("record replay visitor and scope semantics", func(t *testing.T) {
		module, clock := newModule(t, factory)
		ctx := context.Background()
		token := tokenize(t, module, clock.Now(), "visitor-a")
		postA := traffic.Resource{Kind: "post", ID: "post-a"}
		postB := traffic.Resource{Kind: "post", ID: "post-b"}

		first := record(t, module, observation("event-a-00000001", postA, clock.Now(), token, traffic.VisitHuman))
		if !first.Counted || first.Replay || !first.FirstInstanceVisitor || !first.FirstResourceVisitor {
			t.Fatalf("unexpected first record result: %+v", first)
		}
		if first.InstanceTotals != (traffic.Totals{Views: 1, UniqueVisitorDays: 1}) {
			t.Fatalf("unexpected instance totals: %+v", first.InstanceTotals)
		}

		second := record(t, module, observation("event-b-00000001", postA, clock.Now().Add(time.Minute), token, traffic.VisitHuman))
		if !second.Counted || second.Replay || second.FirstInstanceVisitor || second.FirstResourceVisitor {
			t.Fatalf("unexpected repeated visitor result: %+v", second)
		}
		if second.ResourceTotals != (traffic.Totals{Views: 2, UniqueVisitorDays: 1}) {
			t.Fatalf("unexpected resource totals: %+v", second.ResourceTotals)
		}

		otherResource := record(t, module, observation("event-c-00000001", postB, clock.Now().Add(2*time.Minute), token, traffic.VisitHuman))
		if otherResource.FirstInstanceVisitor || !otherResource.FirstResourceVisitor {
			t.Fatalf("visitor must be unique per resource but not twice for the instance: %+v", otherResource)
		}
		if otherResource.InstanceTotals != (traffic.Totals{Views: 3, UniqueVisitorDays: 1}) {
			t.Fatalf("unexpected instance totals after second resource: %+v", otherResource.InstanceTotals)
		}

		replay := record(t, module, observation("event-a-00000001", postA, clock.Now(), token, traffic.VisitHuman))
		if !replay.Replay || replay.InstanceTotals.Views != 3 || replay.ResourceTotals.Views != 2 {
			t.Fatalf("replay must return current snapshots without recounting: %+v", replay)
		}

		_, err := module.Record(ctx, observation("event-a-00000001", postB, clock.Now(), token, traffic.VisitHuman))
		if !traffic.IsKind(err, traffic.ErrorConflict) {
			t.Fatalf("different payload reuse must conflict, got %v", err)
		}
	})

	t.Run("daily boundary and excluded classes", func(t *testing.T) {
		module, clock := newModule(t, factory)
		ctx := context.Background()
		post := traffic.Resource{Kind: "post", ID: "post-a"}
		tokenDayOne := tokenize(t, module, clock.Now(), "visitor-a")
		record(t, module, observation("event-day-one-001", post, clock.Now(), tokenDayOne, traffic.VisitUnknown))

		bot := record(t, module, observation("event-bot-0000001", post, clock.Now(), tokenDayOne, traffic.VisitBot))
		if bot.Counted || bot.DropReason != traffic.DropBot || bot.ResourceTotals.Views != 1 {
			t.Fatalf("bot must be receipted but excluded: %+v", bot)
		}
		internal := record(t, module, observation("event-internal-001", post, clock.Now(), tokenDayOne, traffic.VisitInternal))
		if internal.Counted || internal.DropReason != traffic.DropInternal || internal.ResourceTotals.Views != 1 {
			t.Fatalf("internal traffic must be excluded: %+v", internal)
		}

		// Asia/Shanghai enters 2026-07-24 at 16:00 UTC.
		clock.Set(time.Date(2026, 7, 23, 16, 1, 0, 0, time.UTC))
		tokenDayTwo := tokenize(t, module, clock.Now(), "visitor-a")
		if tokenDayTwo == tokenDayOne {
			t.Fatal("visitor token must change at the catalog day boundary")
		}
		next := record(t, module, observation("event-day-two-001", post, clock.Now(), tokenDayTwo, traffic.VisitHuman))
		if !next.FirstInstanceVisitor || !next.FirstResourceVisitor {
			t.Fatalf("same seed on another day must be a new daily visitor: %+v", next)
		}
		summary, err := module.Summary(ctx, traffic.SummaryQuery{Scope: traffic.ResourceScope(post)})
		if err != nil {
			t.Fatal(err)
		}
		if summary.Totals != (traffic.Totals{Views: 2, UniqueVisitorDays: 2}) {
			t.Fatalf("unexpected all-time summary: %+v", summary)
		}
	})

	t.Run("batch is atomic and ordered", func(t *testing.T) {
		module, clock := newModule(t, factory)
		ctx := context.Background()
		post := traffic.Resource{Kind: "post", ID: "post-a"}
		token := tokenize(t, module, clock.Now(), "visitor-a")
		results, err := module.RecordBatch(ctx, []traffic.Observation{
			observation("event-batch-00001", post, clock.Now(), token, traffic.VisitHuman),
			observation("event-batch-00001", post, clock.Now(), token, traffic.VisitHuman),
			observation("event-batch-00002", post, clock.Now().Add(time.Minute), token, traffic.VisitHuman),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 3 || results[0].Replay || !results[1].Replay || results[2].Replay {
			t.Fatalf("unexpected ordered results: %+v", results)
		}
		before := results[2].ResourceTotals
		_, err = module.RecordBatch(ctx, []traffic.Observation{
			observation("event-batch-00003", post, clock.Now().Add(2*time.Minute), token, traffic.VisitHuman),
			observation("event-batch-00001", traffic.Resource{Kind: "post", ID: "other"}, clock.Now(), token, traffic.VisitHuman),
		})
		if !traffic.IsKind(err, traffic.ErrorConflict) {
			t.Fatalf("batch payload conflict must fail atomically, got %v", err)
		}
		after, err := module.Summary(ctx, traffic.SummaryQuery{Scope: traffic.ResourceScope(post)})
		if err != nil {
			t.Fatal(err)
		}
		if after.Totals != before {
			t.Fatalf("failed batch mutated totals: before=%+v after=%+v", before, after.Totals)
		}
	})

	t.Run("concurrent replay and daily visitor are exact", func(t *testing.T) {
		module, clock := newModule(t, factory)
		ctx := context.Background()
		post := traffic.Resource{Kind: "post", ID: "post-a"}
		token := tokenize(t, module, clock.Now(), "visitor-a")

		const workers = 24
		var wait sync.WaitGroup
		wait.Add(workers)
		results := make(chan traffic.RecordResult, workers)
		failures := make(chan error, workers)
		for range workers {
			go func() {
				defer wait.Done()
				result, err := module.Record(ctx, observation(
					"event-concurrent-replay", post, clock.Now(), token, traffic.VisitHuman,
				))
				if err != nil {
					failures <- err
					return
				}
				results <- result
			}()
		}
		wait.Wait()
		close(results)
		close(failures)
		for err := range failures {
			t.Fatal(err)
		}
		originals := 0
		for result := range results {
			if !result.Replay {
				originals++
			}
		}
		if originals != 1 {
			t.Fatalf("expected one original concurrent event, got %d", originals)
		}

		wait.Add(workers)
		failures = make(chan error, workers)
		for index := range workers {
			go func(index int) {
				defer wait.Done()
				_, err := module.Record(ctx, observation(
					fmt.Sprintf("event-concurrent-%03d", index),
					post, clock.Now().Add(time.Duration(index+1)*time.Second), token, traffic.VisitHuman,
				))
				if err != nil {
					failures <- err
				}
			}(index)
		}
		wait.Wait()
		close(failures)
		for err := range failures {
			t.Fatal(err)
		}
		summary, err := module.Summary(ctx, traffic.SummaryQuery{Scope: traffic.ResourceScope(post)})
		if err != nil {
			t.Fatal(err)
		}
		if summary.Totals != (traffic.Totals{Views: workers + 1, UniqueVisitorDays: 1}) {
			t.Fatalf("concurrent views or unique visitor drifted: %+v", summary.Totals)
		}
	})

	t.Run("baseline queries series totals and ranking", func(t *testing.T) {
		module, clock := newModule(t, factory)
		ctx := context.Background()
		postA := traffic.Resource{Kind: "post", ID: "post-a"}
		postB := traffic.Resource{Kind: "post", ID: "post-b"}
		imported, err := module.ImportBaseline(ctx, traffic.BaselineImport{Source: "post_stats", Resource: postA, Views: 10})
		if err != nil {
			t.Fatal(err)
		}
		if !imported.Applied || imported.Replay || imported.ResourceTotals.Views != 10 {
			t.Fatalf("unexpected baseline result: %+v", imported)
		}
		replay, err := module.ImportBaseline(ctx, traffic.BaselineImport{Source: "post_stats", Resource: postA, Views: 10})
		if err != nil || !replay.Replay || replay.ResourceTotals.Views != 10 {
			t.Fatalf("unexpected baseline replay: %+v err=%v", replay, err)
		}
		_, err = module.ImportBaseline(ctx, traffic.BaselineImport{Source: "post_stats", Resource: postA, Views: 11})
		if !traffic.IsKind(err, traffic.ErrorConflict) {
			t.Fatalf("changed baseline must conflict, got %v", err)
		}

		tokenA := tokenize(t, module, clock.Now(), "visitor-a")
		tokenB := tokenize(t, module, clock.Now(), "visitor-b")
		record(t, module, observation("event-query-00001", postA, clock.Now(), tokenA, traffic.VisitHuman))
		record(t, module, observation("event-query-00002", postB, clock.Now(), tokenB, traffic.VisitHuman))
		record(t, module, observation("event-query-00003", postB, clock.Now().Add(time.Minute), tokenA, traffic.VisitHuman))

		rangeDay := traffic.DateRange{From: traffic.MustParseDay("2026-07-23"), To: traffic.MustParseDay("2026-07-24")}
		rangeSummary, err := module.Summary(ctx, traffic.SummaryQuery{Scope: traffic.ResourceScope(postA), Range: &rangeDay})
		if err != nil {
			t.Fatal(err)
		}
		if rangeSummary.Totals != (traffic.Totals{Views: 1, UniqueVisitorDays: 1}) {
			t.Fatalf("baseline must not fabricate daily history: %+v", rangeSummary)
		}
		allTime, err := module.Summary(ctx, traffic.SummaryQuery{Scope: traffic.ResourceScope(postA)})
		if err != nil {
			t.Fatal(err)
		}
		if allTime.Totals != (traffic.Totals{Views: 11, UniqueVisitorDays: 1}) {
			t.Fatalf("unexpected all-time total: %+v", allTime)
		}
		series, err := module.Series(ctx, traffic.SeriesQuery{
			Scope: traffic.ResourceScope(postA),
			Range: traffic.DateRange{From: traffic.MustParseDay("2026-07-22"), To: traffic.MustParseDay("2026-07-25")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(series) != 3 || series[0].Totals.Views != 0 || series[1].Totals.Views != 1 || series[2].Totals.Views != 0 {
			t.Fatalf("series must include empty days: %+v", series)
		}
		top, err := module.Top(ctx, traffic.TopQuery{ResourceKind: "post", Metric: traffic.RankViews, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(top) != 2 || top[0].Resource != postA || top[0].Totals.Views != 11 || top[1].Resource != postB {
			t.Fatalf("unexpected all-time ranking: %+v", top)
		}
		rangeTop, err := module.Top(ctx, traffic.TopQuery{ResourceKind: "post", Range: &rangeDay, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(rangeTop) != 2 || rangeTop[0].Resource != postB || rangeTop[0].Totals.Views != 2 {
			t.Fatalf("unexpected range ranking: %+v", rangeTop)
		}
		totals, err := module.Totals(ctx, []traffic.Resource{postB, postA, {Kind: "post", ID: "missing"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(totals) != 3 || totals[0].Totals.Views != 2 || totals[1].Totals.Views != 11 || totals[2].Totals.Views != 0 {
			t.Fatalf("batch totals must preserve order: %+v", totals)
		}
	})

	t.Run("forget resource preserves instance aggregate", func(t *testing.T) {
		module, clock := newModule(t, factory)
		ctx := context.Background()
		post := traffic.Resource{Kind: "post", ID: "post-a"}
		token := tokenize(t, module, clock.Now(), "visitor-a")
		record(t, module, observation("event-forget-0001", post, clock.Now(), token, traffic.VisitHuman))
		if _, err := module.ImportBaseline(ctx, traffic.BaselineImport{Source: "post_stats", Resource: post, Views: 9}); err != nil {
			t.Fatal(err)
		}
		result, err := module.ForgetResource(ctx, post)
		if err != nil {
			t.Fatal(err)
		}
		if result.TotalsRemoved != 1 || result.ReceiptsRemoved != 1 || result.BaselinesRemoved != 1 {
			t.Fatalf("unexpected forget result: %+v", result)
		}
		resourceSummary, err := module.Summary(ctx, traffic.SummaryQuery{Scope: traffic.ResourceScope(post)})
		if err != nil {
			t.Fatal(err)
		}
		instanceSummary, err := module.Summary(ctx, traffic.SummaryQuery{Scope: traffic.InstanceScope()})
		if err != nil {
			t.Fatal(err)
		}
		if resourceSummary.Totals != (traffic.Totals{}) {
			t.Fatalf("resource data was not forgotten: %+v", resourceSummary)
		}
		if instanceSummary.Totals != (traffic.Totals{Views: 10, UniqueVisitorDays: 1}) {
			t.Fatalf("instance aggregates must be preserved: %+v", instanceSummary)
		}
	})

	t.Run("prune removes bounded identity state", func(t *testing.T) {
		module, clock := newModule(t, factory)
		post := traffic.Resource{Kind: "post", ID: "post-a"}
		token := tokenize(t, module, clock.Now(), "visitor-a")
		record(t, module, observation("event-prune-00001", post, clock.Now(), token, traffic.VisitHuman))
		result, err := module.Prune(context.Background(), clock.Now().Add(10*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if result.ReceiptsRemoved != 1 || result.VisitorMarkersRemoved != 2 {
			t.Fatalf("expected one receipt and two scope markers removed, got %+v", result)
		}
		summary, err := module.Summary(context.Background(), traffic.SummaryQuery{Scope: traffic.ResourceScope(post)})
		if err != nil {
			t.Fatal(err)
		}
		if summary.Totals != (traffic.Totals{Views: 1, UniqueVisitorDays: 1}) {
			t.Fatalf("prune must preserve aggregates: %+v", summary)
		}
	})
}

func newModule(t *testing.T, factory Factory) (traffic.Module, *Clock) {
	t.Helper()
	clock := NewClock(time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))
	catalog, err := traffic.Compile(traffic.Definition{
		Version:  traffic.DefinitionVersion,
		TimeZone: "Asia/Shanghai",
		ResourceKinds: []traffic.ResourceKindDefinition{
			{Key: "post"},
			{Key: "asset"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return factory(t, catalog, clock), clock
}

func tokenize(t *testing.T, module traffic.Module, at time.Time, seed string) traffic.VisitorToken {
	t.Helper()
	token, err := module.TokenizeVisitor(context.Background(), at, []byte(seed))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func observation(id string, resource traffic.Resource, at time.Time, token traffic.VisitorToken, class traffic.VisitClass) traffic.Observation {
	return traffic.Observation{
		EventID: traffic.EventID(id), Resource: resource, OccurredAt: at, Class: class,
		HasVisitor: true, VisitorToken: token,
	}
}

func record(t *testing.T, module traffic.Module, value traffic.Observation) traffic.RecordResult {
	t.Helper()
	result, err := module.Record(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
