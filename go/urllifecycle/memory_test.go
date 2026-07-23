package urllifecycle

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func route(kind ResourceKind, id, variant string) RouteKey {
	return RouteKey{Resource: ResourceKey{Kind: kind, ID: id}, Variant: variant}
}

func local(path string, query ...QueryValue) LocalRef {
	return LocalRef{Path: path, Query: query}
}

func testMemory(t *testing.T, now *time.Time) *Memory {
	t.Helper()
	memory, err := NewMemory(testCatalog(t), MemoryOptions{Clock: func() time.Time { return *now }})
	if err != nil {
		t.Fatal(err)
	}
	return memory
}

func apply(t *testing.T, memory *Memory, set ChangeSet) Receipt {
	t.Helper()
	receipt, err := memory.Apply(context.Background(), set, ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestDocsVariantsCanOwnTheSamePath(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	en := route("doc", "guide-en", "en:default")
	zh := route("doc", "guide-zh", "zh-CN:default")
	apply(t, memory, Claim(MutationMeta{CommandID: "claim", Reason: "backfill"},
		ClaimSpec{Route: en, Active: ActiveRoute{Canonical: local("/guide")}},
		ClaimSpec{Route: zh, Active: ActiveRoute{Canonical: local(
			"/guide", QueryValue{Key: "locale", Value: "zh-CN"},
		)}},
	))

	defaultResolution, err := memory.Resolve(context.Background(), Lookup{EscapedPath: "/guide"})
	if err != nil {
		t.Fatal(err)
	}
	if defaultResolution.Kind != ResolutionCanonical || defaultResolution.Route == nil || *defaultResolution.Route != en {
		t.Fatalf("default variant resolved incorrectly: %#v", defaultResolution)
	}
	zhResolution, err := memory.Resolve(context.Background(), Lookup{
		EscapedPath: "/guide", RawQuery: "locale=zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if zhResolution.Kind != ResolutionCanonical || zhResolution.Route == nil || *zhResolution.Route != zh {
		t.Fatalf("zh variant resolved incorrectly: %#v", zhResolution)
	}
}

func TestRepeatedRenameAlwaysResolvesInOneHop(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	post := route("page", "post-1", "")
	active := ActiveRoute{Canonical: local("/posts/old")}
	first := apply(t, memory, Claim(MutationMeta{CommandID: "create", Reason: "publish"},
		ClaimSpec{Route: post, Active: active},
	))
	active.Canonical = local("/posts/new")
	second := apply(t, memory, Rename(
		MutationMeta{CommandID: "rename-1", Reason: "rename"},
		post, first.RouteRevisions[0].Revision, ActiveRoute{Canonical: local("/posts/old")},
		local("/posts/new"), DefaultPermanentRedirect(),
	))
	_ = second
	third := apply(t, memory, Rename(
		MutationMeta{CommandID: "rename-2", Reason: "rename again"},
		post, 2, active, local("/posts/latest"), DefaultPermanentRedirect(),
	))
	if third.Revision != 3 {
		t.Fatalf("unexpected revision %d", third.Revision)
	}
	for _, old := range []string{"/posts/old", "/posts/new"} {
		resolution, err := memory.Resolve(context.Background(), Lookup{
			EscapedPath: old, RawQuery: "utm_source=test",
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolution.Kind != ResolutionRedirect || resolution.StatusCode != 308 ||
			resolution.Location != "/posts/latest?utm_source=test" {
			t.Fatalf("%s did not resolve directly to terminal canonical: %#v", old, resolution)
		}
	}
}

func TestRenamePreservesTargetIdentityAndOnlyExtraQuery(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	doc := route("doc", "doc-1", "zh-CN:v2")
	old := local("/guide/install",
		QueryValue{Key: "locale", Value: "zh-CN"},
		QueryValue{Key: "version", Value: "v2"},
	)
	created := apply(t, memory, Claim(MutationMeta{CommandID: "create", Reason: "publish"},
		ClaimSpec{Route: doc, Active: ActiveRoute{Canonical: old}},
	))
	next := local("/manual/install",
		QueryValue{Key: "locale", Value: "zh-CN"},
		QueryValue{Key: "version", Value: "v2"},
	)
	apply(t, memory, Rename(
		MutationMeta{CommandID: "move", Reason: "move docs"},
		doc, created.RouteRevisions[0].Revision, ActiveRoute{Canonical: old},
		next, DefaultPermanentRedirect(),
	))
	resolution, err := memory.Resolve(context.Background(), Lookup{
		EscapedPath: "/guide/install",
		RawQuery:    "version=v2&utm_source=x&locale=zh-CN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Location != "/manual/install?locale=zh-CN&version=v2&utm_source=x" {
		t.Fatalf("identity query was not replaced canonically: %q", resolution.Location)
	}
}

func TestGoneUnknownAndExplicitReleaseRemainDistinct(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	firstRoute := route("page", "gone", "")
	secondRoute := route("page", "released", "")
	created := apply(t, memory, Claim(MutationMeta{CommandID: "create", Reason: "publish"},
		ClaimSpec{Route: firstRoute, Active: ActiveRoute{Canonical: local("/gone")}},
		ClaimSpec{Route: secondRoute, Active: ActiveRoute{Canonical: local("/released")}},
	))
	revisions := map[string]RouteRevision{}
	for _, item := range created.RouteRevisions {
		revisions[item.Route.Resource.ID] = item.Revision
	}
	apply(t, memory, Retire(MutationMeta{CommandID: "retire", Reason: "remove"},
		RetireGone(firstRoute, revisions["gone"]),
		ReleaseRoute(secondRoute, revisions["released"]),
	))
	gone, err := memory.Resolve(context.Background(), Lookup{EscapedPath: "/gone"})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := memory.Resolve(context.Background(), Lookup{EscapedPath: "/released"})
	if err != nil {
		t.Fatal(err)
	}
	if gone.Kind != ResolutionGone || unknown.Kind != ResolutionUnknown {
		t.Fatalf("gone/released drifted: gone=%s released=%s", gone.Kind, unknown.Kind)
	}
}

func TestTemporaryOverlayExpiresBackToCanonical(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	source := route("page", "source", "")
	target := route("page", "target", "")
	created := apply(t, memory, Claim(MutationMeta{CommandID: "create", Reason: "publish"},
		ClaimSpec{Route: source, Active: ActiveRoute{Canonical: local("/source")}},
		ClaimSpec{Route: target, Active: ActiveRoute{Canonical: local("/target")}},
	))
	sourceRevision := RouteRevision(0)
	for _, item := range created.RouteRevisions {
		if item.Route == source {
			sourceRevision = item.Revision
		}
	}
	expires := now.Add(time.Hour)
	apply(t, memory, SetTemporaryRedirect(MutationMeta{CommandID: "overlay", Reason: "maintenance"},
		OverlayChange{
			Owner: source, Source: local("/source"), ExpectedRevision: sourceRevision,
			Desired: &TemporaryRedirect{
				Target: RouteTarget(target), Policy: DefaultTemporaryRedirect(), ExpiresAt: &expires,
			},
		},
	))
	redirected, err := memory.Resolve(context.Background(), Lookup{EscapedPath: "/source"})
	if err != nil {
		t.Fatal(err)
	}
	if redirected.Kind != ResolutionRedirect || redirected.StatusCode != 307 || redirected.Location != "/target" {
		t.Fatalf("temporary overlay did not redirect: %#v", redirected)
	}
	now = expires
	canonical, err := memory.Resolve(context.Background(), Lookup{EscapedPath: "/source"})
	if err != nil {
		t.Fatal(err)
	}
	if canonical.Kind != ResolutionCanonical {
		t.Fatalf("expired overlay did not reveal base canonical: %#v", canonical)
	}
}

func TestBatchConflictIsAtomic(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	_, err := memory.Apply(context.Background(), Claim(
		MutationMeta{CommandID: "conflict", Reason: "bad batch"},
		ClaimSpec{Route: route("page", "one", ""), Active: ActiveRoute{Canonical: local("/same")}},
		ClaimSpec{Route: route("page", "two", ""), Active: ActiveRoute{Canonical: local("/same")}},
	), ApplyOptions{})
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	resolution, resolveErr := memory.Resolve(context.Background(), Lookup{EscapedPath: "/same"})
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if resolution.Kind != ResolutionUnknown {
		t.Fatalf("failed batch partially applied: %#v", resolution)
	}
}

func TestIdempotencyReplayAndConflict(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	set := Claim(MutationMeta{CommandID: "same-command", Reason: "publish"},
		ClaimSpec{Route: route("page", "one", ""), Active: ActiveRoute{Canonical: local("/one")}},
	)
	first := apply(t, memory, set)
	replay := apply(t, memory, set)
	if !replay.Replay || replay.Revision != first.Revision {
		t.Fatalf("idempotent replay changed receipt: first=%#v replay=%#v", first, replay)
	}
	changed := set
	changed.ResourceChanges[0].Desired.Canonical = local("/two")
	_, err := memory.Apply(context.Background(), changed, ApplyOptions{})
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorIdempotencyConflict {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestArchiveRoundTripPreservesGoneAndRedirect(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	source := testMemory(t, &now)
	key := route("page", "post", "")
	created := apply(t, source, Claim(MutationMeta{CommandID: "create", Reason: "publish"},
		ClaimSpec{Route: key, Active: ActiveRoute{Canonical: local("/old")}},
	))
	apply(t, source, Rename(MutationMeta{CommandID: "rename", Reason: "rename"},
		key, created.RouteRevisions[0].Revision, ActiveRoute{Canonical: local("/old")},
		local("/new"), DefaultPermanentRedirect(),
	))
	var archive bytes.Buffer
	manifest, err := source.Export(context.Background(), ExportQuery{IncludeAudit: true}, &archive)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Records == 0 {
		t.Fatal("empty archive")
	}
	target := testMemory(t, &now)
	report, err := target.Restore(context.Background(), RestoreCommand{
		RequireEmpty: true, CommandID: "restore", Reason: "restore test",
	}, bytes.NewReader(archive.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if report.Receipt == nil {
		t.Fatal("restore did not commit")
	}
	resolution, err := target.Resolve(context.Background(), Lookup{EscapedPath: "/old"})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Kind != ResolutionRedirect || resolution.Location != "/new" {
		t.Fatalf("archive lost redirect: %#v", resolution)
	}
}

func TestDanglingTargetAndSelfOverlayAreRejected(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	source := route("page", "source", "")
	created := apply(t, memory, Claim(MutationMeta{CommandID: "create", Reason: "publish"},
		ClaimSpec{Route: source, Active: ActiveRoute{Canonical: local("/source")}},
	))
	_, err := memory.Apply(context.Background(), SetTemporaryRedirect(
		MutationMeta{CommandID: "self-loop", Reason: "invalid"},
		OverlayChange{
			Owner: source, Source: local("/source"),
			ExpectedRevision: created.RouteRevisions[0].Revision,
			Desired: &TemporaryRedirect{
				Target: RouteTarget(source), Policy: DefaultTemporaryRedirect(),
			},
		},
	), ApplyOptions{})
	var typed *Error
	if !errors.As(err, &typed) || typed.Kind != ErrorCycle {
		t.Fatalf("expected cycle error, got %v", err)
	}
	_, err = memory.Apply(context.Background(), SetTemporaryRedirect(
		MutationMeta{CommandID: "dangling", Reason: "invalid"},
		OverlayChange{
			Owner: source, Source: local("/source"),
			ExpectedRevision: created.RouteRevisions[0].Revision,
			Desired: &TemporaryRedirect{
				Target: RouteTarget(route("page", "missing", "")),
				Policy: DefaultTemporaryRedirect(),
			},
		},
	), ApplyOptions{})
	if !errors.As(err, &typed) || typed.Kind != ErrorDanglingTarget {
		t.Fatalf("expected dangling target, got %v", err)
	}
}

func TestConcurrentRenameUsesRouteRevision(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	memory := testMemory(t, &now)
	key := route("page", "post", "")
	created := apply(t, memory, Claim(MutationMeta{CommandID: "create", Reason: "publish"},
		ClaimSpec{Route: key, Active: ActiveRoute{Canonical: local("/start")}},
	))
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for index, path := range []string{"/left", "/right"} {
		wait.Add(1)
		go func(index int, path string) {
			defer wait.Done()
			_, err := memory.Apply(context.Background(), Rename(
				MutationMeta{CommandID: CommandID("rename-" + path), Reason: "concurrent rename"},
				key, created.RouteRevisions[0].Revision,
				ActiveRoute{Canonical: local("/start")}, local(path),
				DefaultPermanentRedirect(),
			), ApplyOptions{})
			results <- err
		}(index, path)
	}
	wait.Wait()
	close(results)
	successes, stale := 0, 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var typed *Error
		if errors.As(err, &typed) && typed.Kind == ErrorStaleRevision {
			stale++
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent rename results: success=%d stale=%d", successes, stale)
	}
}
