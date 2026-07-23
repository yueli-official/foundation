package discovery

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testModule(t *testing.T) *Module {
	t.Helper()
	return MustCompile(Definition{
		ContractVersion: ContractVersion,
		Site: SiteProfile{
			Origin: "https://example.com", Name: "Example",
			Description: "Example site", DefaultLocale: "zh-CN",
			DefaultImage: &Image{URL: "/default.png", Alt: "Example"},
		},
	})
}

func TestCompileRejectsUntrustedOriginShape(t *testing.T) {
	_, err := Compile(Definition{
		ContractVersion: ContractVersion,
		Site: SiteProfile{
			Origin: "https://example.com/from/request", Name: "Example",
			DefaultLocale: "zh-CN",
		},
	})
	if !IsKind(err, ErrorConfiguration) {
		t.Fatalf("expected configuration error, got %v", err)
	}
}

func TestProjectDerivesOneCanonicalIdentity(t *testing.T) {
	module := testModule(t)
	published := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	projection, report, err := module.Project(PageDescriptor{
		Key: "post:one", Path: "/posts/one",
		Subject: ArticleSubject(Article{
			Title: "One", Description: "First", PublishedAt: published,
			Authors: []Person{{Name: "Author"}},
		}),
		Alternates: []Alternate{{Locale: "en", Path: "/en/posts/one"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", report.Diagnostics)
	}
	if projection.CanonicalURL != "https://example.com/posts/one" {
		t.Fatalf("unexpected canonical: %s", projection.CanonicalURL)
	}
	if projection.Sitemap == nil || projection.Sitemap.Location != projection.CanonicalURL {
		t.Fatalf("sitemap does not share canonical: %#v", projection.Sitemap)
	}
	encoded, err := json.Marshal(projection.Head.StructuredData[0].JSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `https://example.com/posts/one`) {
		t.Fatalf("JSON-LD does not contain canonical: %s", encoded)
	}
	var ogURL string
	for _, meta := range projection.Head.Meta {
		if meta.Property == "og:url" {
			ogURL = meta.Content
		}
	}
	if ogURL != projection.CanonicalURL {
		t.Fatalf("Open Graph URL drifted: %q", ogURL)
	}
}

func TestProjectUnlistedCannotEnterSitemap(t *testing.T) {
	module := testModule(t)
	projection, _, err := module.Project(PageDescriptor{
		Key: "search", Path: "/search", Visibility: Unlisted,
		Subject: WebPageSubject(WebPage{Title: "Search"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Sitemap != nil {
		t.Fatal("unlisted page entered sitemap")
	}
	if projection.Headers.XRobotsTag != "noindex,follow" {
		t.Fatalf("unexpected robots projection: %s", projection.Headers.XRobotsTag)
	}
}

func TestProjectKeepsFollowSeparateFromIndexability(t *testing.T) {
	module := testModule(t)
	projection, _, err := module.Project(PageDescriptor{
		Key: "archive", Path: "/archive", Visibility: Discoverable, Follow: NoFollow,
		Subject: WebPageSubject(WebPage{Title: "Archive"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Sitemap == nil || projection.Headers.XRobotsTag != "index,nofollow" {
		t.Fatalf("index and follow policy were conflated: %#v", projection)
	}
}

func TestProjectRejectsCrossOriginAndScriptEscapeIsSafe(t *testing.T) {
	module := testModule(t)
	_, _, err := module.Project(PageDescriptor{
		Key: "bad", Path: "https://evil.example/bad",
		Subject: WebPageSubject(WebPage{Title: "Bad"}),
	})
	if !IsKind(err, ErrorConflict) {
		t.Fatalf("expected origin conflict, got %v", err)
	}
	projection, _, err := module.Project(PageDescriptor{
		Key: "safe", Path: "/safe",
		Subject: WebPageSubject(WebPage{
			Title: `</script><script>alert(1)</script>`,
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(projection.Head.StructuredData[0].JSON), "</script>") {
		t.Fatalf("unsafe JSON-LD: %s", projection.Head.StructuredData[0].JSON)
	}
}
