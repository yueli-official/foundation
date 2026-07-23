package discovery

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPublishProducesAtomicProtocolSet(t *testing.T) {
	module := testModule(t)
	published := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	page := PageDescriptor{
		Key: "post:one", Path: "/posts/one",
		Subject: ArticleSubject(Article{
			Title: "One", Description: "First", PublishedAt: published,
		}),
	}
	source := MemorySource{Records: []Record{{
		SortKey: "https://example.com/posts/one", Page: page,
		Feed: &FeedEntryFacts{
			ID: "urn:post:one", Title: "One", PublishedAt: published,
			ContentHTML: `<p>Hello</p><script>alert(1)</script>`,
		},
	}}}
	target := &MemoryTarget{}
	manifest, _, err := module.Publish(context.Background(), PublicationPlan{
		Sitemap: &SitemapPlan{Source: "pages"},
		Feeds: []FeedPlan{
			{
				ID: "urn:feed:posts", Source: "pages", Format: FeedRSS,
				Route: "/feeds/posts.xml", Title: "Posts", UpdatedAt: published,
			},
			{
				ID: "urn:feed:posts", Source: "pages", Format: FeedAtom,
				Route: "/feeds/posts.atom", Title: "Posts", UpdatedAt: published,
			},
		},
		Robots: &RobotsPlan{},
	}, Sources{"pages": source}, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Artifacts) != 5 {
		t.Fatalf("expected 5 artifacts, got %#v", manifest.Artifacts)
	}
	snapshot := target.Snapshot()
	for _, name := range []string{
		"sitemap.xml", "sitemap-00001.xml", "feeds/posts.xml",
		"feeds/posts.atom", "robots.txt",
	} {
		if len(snapshot.Artifacts[name]) == 0 {
			t.Fatalf("artifact %q is missing", name)
		}
	}
	if strings.Contains(string(snapshot.Artifacts["feeds/posts.xml"]), "<script") {
		t.Fatal("feed content was not sanitized")
	}
	if !strings.Contains(string(snapshot.Artifacts["robots.txt"]), "Sitemap: https://example.com/sitemap.xml") {
		t.Fatalf("robots does not advertise sitemap: %s", snapshot.Artifacts["robots.txt"])
	}
}

type failingSource struct{}

func (failingSource) Next(context.Context, Cursor, int) (Batch, error) {
	return Batch{}, errors.New("database unavailable")
}

func TestPublishFailureDoesNotReplaceCommittedArtifacts(t *testing.T) {
	module := testModule(t)
	target := &MemoryTarget{}
	_, _, err := module.Publish(context.Background(), PublicationPlan{
		Robots: &RobotsPlan{},
	}, nil, target)
	if err != nil {
		t.Fatal(err)
	}
	before := target.Snapshot()
	_, _, err = module.Publish(context.Background(), PublicationPlan{
		Sitemap: &SitemapPlan{Source: "broken"},
	}, Sources{"broken": failingSource{}}, target)
	if !IsKind(err, ErrorSource) {
		t.Fatalf("expected source error, got %v", err)
	}
	after := target.Snapshot()
	if string(before.Artifacts["robots.txt"]) != string(after.Artifacts["robots.txt"]) {
		t.Fatal("failed publication replaced committed artifacts")
	}
}

func TestPublishRejectsNonAdvancingCursor(t *testing.T) {
	module := testModule(t)
	source := cursorSourceFunc(func(context.Context, Cursor, int) (Batch, error) {
		return Batch{
			Records: []Record{{
				SortKey: "https://example.com/a",
				Page: PageDescriptor{
					Key: "a", Path: "/a",
					Subject: WebPageSubject(WebPage{Title: "A"}),
				},
			}},
			Done: false,
		}, nil
	})
	_, _, err := module.Publish(context.Background(), PublicationPlan{
		Sitemap: &SitemapPlan{Source: "pages"},
	}, Sources{"pages": source}, &MemoryTarget{})
	if !IsKind(err, ErrorConflict) {
		t.Fatalf("expected cursor conflict, got %v", err)
	}
}

func TestPublishSitemapSplitsAtURLLimitAndIsDeterministic(t *testing.T) {
	module := MustCompile(Definition{
		ContractVersion: ContractVersion,
		Site: SiteProfile{
			Origin: "https://example.com", Name: "Example", DefaultLocale: "en",
		},
		Limits: Limits{MaxSitemapURLs: 1},
	})
	source := MemorySource{Records: []Record{
		sitemapRecord("/a"),
		sitemapRecord("/b"),
	}}
	publish := func() MemoryPublication {
		target := &MemoryTarget{}
		if _, _, err := module.Publish(context.Background(), PublicationPlan{
			Sitemap: &SitemapPlan{Source: "pages"},
		}, Sources{"pages": source}, target); err != nil {
			t.Fatal(err)
		}
		return target.Snapshot()
	}
	first := publish()
	second := publish()
	for _, name := range []string{"sitemap.xml", "sitemap-00001.xml", "sitemap-00002.xml"} {
		if len(first.Artifacts[name]) == 0 {
			t.Fatalf("missing split artifact %q", name)
		}
		if !bytes.Equal(first.Artifacts[name], second.Artifacts[name]) {
			t.Fatalf("artifact %q is not deterministic", name)
		}
	}
	if !reflect.DeepEqual(first.Manifest, second.Manifest) {
		t.Fatalf("manifest is not deterministic:\n%#v\n%#v", first.Manifest, second.Manifest)
	}
}

func TestPublishSitemapSplitsAtByteLimit(t *testing.T) {
	module := MustCompile(Definition{
		ContractVersion: ContractVersion,
		Site: SiteProfile{
			Origin: "https://example.com", Name: "Example", DefaultLocale: "en",
		},
		Limits: Limits{MaxSitemapBytes: 600},
	})
	recordA := sitemapRecord("/a")
	recordB := sitemapRecord("/b")
	for index := range 2 {
		recordA.Page.Alternates = append(recordA.Page.Alternates, Alternate{
			Locale: "locale-" + strings.Repeat("a", 24) + string(rune('a'+index)),
			Path:   "/alternate/" + strings.Repeat("a", 24) + string(rune('a'+index)),
		})
		recordB.Page.Alternates = append(recordB.Page.Alternates, Alternate{
			Locale: "locale-" + strings.Repeat("b", 24) + string(rune('a'+index)),
			Path:   "/alternate/" + strings.Repeat("b", 24) + string(rune('a'+index)),
		})
	}
	target := &MemoryTarget{}
	_, _, err := module.Publish(context.Background(), PublicationPlan{
		Sitemap: &SitemapPlan{Source: "pages"},
	}, Sources{"pages": MemorySource{Records: []Record{recordA, recordB}}}, target)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := target.Snapshot()
	if len(snapshot.Artifacts["sitemap-00001.xml"]) > 600 ||
		len(snapshot.Artifacts["sitemap-00002.xml"]) > 600 {
		t.Fatal("sitemap part exceeded byte limit")
	}
	if len(snapshot.Artifacts["sitemap-00002.xml"]) == 0 {
		t.Fatal("byte limit did not split sitemap")
	}
}

func TestPublishSitemapPartCapacityAbortsAtomically(t *testing.T) {
	module := MustCompile(Definition{
		ContractVersion: ContractVersion,
		Site: SiteProfile{
			Origin: "https://example.com", Name: "Example", DefaultLocale: "en",
		},
		Limits: Limits{MaxSitemapURLs: 1, MaxSitemapParts: 1},
	})
	target := &MemoryTarget{}
	_, _, err := module.Publish(context.Background(), PublicationPlan{
		Sitemap: &SitemapPlan{Source: "pages"},
	}, Sources{"pages": MemorySource{Records: []Record{
		sitemapRecord("/a"),
		sitemapRecord("/b"),
	}}}, target)
	if !IsKind(err, ErrorCapacity) {
		t.Fatalf("expected capacity error, got %v", err)
	}
	if len(target.Snapshot().Artifacts) != 0 {
		t.Fatal("failed split exposed a partial publication")
	}
}

func sitemapRecord(pagePath string) Record {
	return Record{
		SortKey: "https://example.com" + pagePath,
		Page: PageDescriptor{
			Key: pagePath, Path: pagePath,
			Subject: WebPageSubject(WebPage{Title: pagePath}),
		},
	}
}

type cursorSourceFunc func(context.Context, Cursor, int) (Batch, error)

func (function cursorSourceFunc) Next(ctx context.Context, cursor Cursor, limit int) (Batch, error) {
	return function(ctx, cursor, limit)
}
