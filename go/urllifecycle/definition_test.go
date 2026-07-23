package urllifecycle

import (
	"errors"
	"testing"
)

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := Compile(Definition{
		Version:       DefinitionVersion,
		TrustedOrigin: "https://docs.example.test",
		ResourceKinds: []ResourceKindDefinition{
			{Key: "doc"},
			{Key: "page"},
		},
		Namespaces: []NamespaceDefinition{{
			Key: "public", PathPrefix: "/",
			IdentityQuery: QueryIdentityDefinition{Keys: []QueryKeyDefinition{
				{Key: "locale", Default: "en", OmitDefault: true},
				{Key: "version", Default: "default", OmitDefault: true},
			}},
		}},
		ExternalOrigins: []string{"https://archive.example.test:443"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestCompileCanonicalizesPolicy(t *testing.T) {
	first := testCatalog(t)
	second, err := Compile(Definition{
		Version:       DefinitionVersion,
		TrustedOrigin: "https://docs.example.test:443/",
		ResourceKinds: []ResourceKindDefinition{{Key: "page"}, {Key: "doc"}},
		Namespaces: []NamespaceDefinition{{
			Key: "public", PathPrefix: "/",
			IdentityQuery: QueryIdentityDefinition{
				MaxBytes: 4096,
				Keys: []QueryKeyDefinition{
					{Key: "locale", Default: "en", OmitDefault: true},
					{Key: "version", Default: "default", OmitDefault: true},
				},
			},
		}},
		ExternalOrigins: []string{"https://archive.example.test"},
		Limits: Limits{
			MaxPathBytes: 4096, MaxQueryBytes: 4096, MaxResourceIDBytes: 200,
			MaxVariantBytes: 200, MaxReasonBytes: 2000, MaxChanges: 50_000,
			MaxAliasesPerRoute: 100, MaxDiagnostics: 1000, MaxPageSize: 200,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() != second.Digest() {
		t.Fatalf("equivalent definitions have different digests: %s != %s", first.Digest(), second.Digest())
	}
}

func TestCompileRejectsUntrustedOriginShapes(t *testing.T) {
	for _, value := range []string{
		"https://user@example.test",
		"https://example.test/path",
		"javascript:alert(1)",
	} {
		_, err := Compile(Definition{
			Version: DefinitionVersion, TrustedOrigin: value,
			ResourceKinds: []ResourceKindDefinition{{Key: "page"}},
			Namespaces:    []NamespaceDefinition{{Key: "public", PathPrefix: "/"}},
		})
		var typed *Error
		if !errors.As(err, &typed) || typed.Kind != ErrorInvalidInput {
			t.Fatalf("%q: expected invalid input, got %v", value, err)
		}
	}
}

func TestNormalizePathRFCBoundaries(t *testing.T) {
	tests := map[string]string{
		"/a/%7euser":    "/a/~user",
		"/a/%2E%2e/b":   "/b",
		"/a/./b/../c":   "/a/c",
		"/A//b/":        "/A//b/",
		"/技术/%e4%b8%ad": "/技术/%E4%B8%AD",
		"/%2f%2fevil":   "/%2F%2Fevil",
	}
	for input, want := range tests {
		got, err := normalizePath(input, 4096)
		if err != nil {
			t.Fatalf("%q: %v", input, err)
		}
		if got != want {
			t.Fatalf("%q: got %q, want %q", input, got, want)
		}
		again, err := normalizePath(got, 4096)
		if err != nil || again != got {
			t.Fatalf("%q is not idempotent: %q, %v", input, again, err)
		}
	}
	for _, input := range []string{
		"relative", "//evil.test/path", "/a\\b", "/a?x=1", "/a#x",
		"/%", "/%00",
	} {
		if _, err := normalizePath(input, 4096); err == nil {
			t.Fatalf("%q: expected rejection", input)
		}
	}
}

func TestIdentityQueryDefaultsAndExtras(t *testing.T) {
	catalog := testCatalog(t)
	defaultRef, err := catalog.normalizeLookup(Lookup{
		EscapedPath: "/guide", RawQuery: "utm_source=x",
	})
	if err != nil {
		t.Fatal(err)
	}
	explicitDefault, err := catalog.normalizeLocalRef(LocalRef{
		Path:  "/guide",
		Query: []QueryValue{{Key: "locale", Value: "en"}, {Key: "version", Value: "default"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if defaultRef.ref.key != explicitDefault.key {
		t.Fatal("default identity query did not normalize to the same key")
	}
	if defaultRef.ref.query != "" || defaultRef.extras != "utm_source=x" {
		t.Fatalf("unexpected canonical/default query split: %#v", defaultRef)
	}
	_, err = catalog.normalizeLookup(Lookup{
		EscapedPath: "/guide", RawQuery: "locale=en&locale=zh-CN",
	})
	if err == nil {
		t.Fatal("duplicate identity key was accepted")
	}
}
