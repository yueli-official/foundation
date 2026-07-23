// Package urllifecycletest contains reusable behavioral conformance for URL
// Lifecycle Adapters.
package urllifecycletest

import (
	"context"
	"errors"
	"testing"

	"github.com/yueli-official/foundation/go/urllifecycle"
)

type Factory func(*testing.T, *urllifecycle.Catalog) urllifecycle.Module

func Definition() urllifecycle.Definition {
	return urllifecycle.Definition{
		Version:       urllifecycle.DefinitionVersion,
		TrustedOrigin: "https://conformance.example.test",
		ResourceKinds: []urllifecycle.ResourceKindDefinition{{Key: "page"}, {Key: "doc"}},
		Namespaces: []urllifecycle.NamespaceDefinition{{
			Key: "public", PathPrefix: "/",
			IdentityQuery: urllifecycle.QueryIdentityDefinition{Keys: []urllifecycle.QueryKeyDefinition{
				{Key: "locale", Default: "en", OmitDefault: true},
				{Key: "version", Default: "default", OmitDefault: true},
			}},
		}},
	}
}

func Run(t *testing.T, factory Factory) {
	t.Helper()
	catalog, err := urllifecycle.Compile(Definition())
	if err != nil {
		t.Fatal(err)
	}
	module := factory(t, catalog)
	ctx := context.Background()
	post := route("page", "post", "")
	other := route("page", "other", "")

	t.Run("claim_and_variant_resolution", func(t *testing.T) {
		receipt, err := module.Apply(ctx, urllifecycle.Claim(
			meta("claim"),
			urllifecycle.ClaimSpec{
				Route:  post,
				Active: urllifecycle.ActiveRoute{Canonical: urllifecycle.LocalRef{Path: "/post"}},
			},
			urllifecycle.ClaimSpec{
				Route: route("doc", "guide-zh", "zh-CN"),
				Active: urllifecycle.ActiveRoute{Canonical: urllifecycle.LocalRef{
					Path:  "/guide",
					Query: []urllifecycle.QueryValue{{Key: "locale", Value: "zh-CN"}},
				}},
			},
		), urllifecycle.ApplyOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Revision == 0 {
			t.Fatal("claim did not advance revision")
		}
		resolution, err := module.Resolve(ctx, urllifecycle.Lookup{
			EscapedPath: "/guide", RawQuery: "locale=zh-CN",
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolution.Kind != urllifecycle.ResolutionCanonical ||
			resolution.Route == nil || resolution.Route.Variant != "zh-CN" {
			t.Fatalf("variant resolution drifted: %#v", resolution)
		}
	})

	t.Run("owner_target_is_single_hop", func(t *testing.T) {
		inspection, err := module.Inspect(ctx, urllifecycle.InspectQuery{Route: &post})
		if err != nil {
			t.Fatal(err)
		}
		current := *inspection.Active
		_, err = module.Apply(ctx, urllifecycle.Rename(
			meta("rename-1"), post, inspection.Revision, current,
			urllifecycle.LocalRef{Path: "/post-new"},
			urllifecycle.DefaultPermanentRedirect(),
		), urllifecycle.ApplyOptions{})
		if err != nil {
			t.Fatal(err)
		}
		inspection, err = module.Inspect(ctx, urllifecycle.InspectQuery{Route: &post})
		if err != nil {
			t.Fatal(err)
		}
		_, err = module.Apply(ctx, urllifecycle.Rename(
			meta("rename-2"), post, inspection.Revision, *inspection.Active,
			urllifecycle.LocalRef{Path: "/post-final"},
			urllifecycle.DefaultPermanentRedirect(),
		), urllifecycle.ApplyOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/post", "/post-new"} {
			resolution, err := module.Resolve(ctx, urllifecycle.Lookup{EscapedPath: path})
			if err != nil {
				t.Fatal(err)
			}
			if resolution.Kind != urllifecycle.ResolutionRedirect ||
				resolution.Location != "/post-final" {
				t.Fatalf("%s did not resolve in one hop: %#v", path, resolution)
			}
		}
	})

	t.Run("atomic_conflict", func(t *testing.T) {
		_, err := module.Apply(ctx, urllifecycle.Claim(
			meta("conflict"),
			urllifecycle.ClaimSpec{
				Route:  other,
				Active: urllifecycle.ActiveRoute{Canonical: urllifecycle.LocalRef{Path: "/post-final"}},
			},
		), urllifecycle.ApplyOptions{})
		var typed *urllifecycle.Error
		if !errors.As(err, &typed) || typed.Kind != urllifecycle.ErrorConflict {
			t.Fatalf("expected typed conflict, got %v", err)
		}
	})

	t.Run("gone_is_not_unknown", func(t *testing.T) {
		inspection, err := module.Inspect(ctx, urllifecycle.InspectQuery{Route: &post})
		if err != nil {
			t.Fatal(err)
		}
		_, err = module.Apply(ctx, urllifecycle.Retire(
			meta("gone"),
			urllifecycle.RetireGone(post, inspection.Revision),
		), urllifecycle.ApplyOptions{})
		if err != nil {
			t.Fatal(err)
		}
		gone, err := module.Resolve(ctx, urllifecycle.Lookup{EscapedPath: "/post-final"})
		if err != nil {
			t.Fatal(err)
		}
		unknown, err := module.Resolve(ctx, urllifecycle.Lookup{EscapedPath: "/never-seen"})
		if err != nil {
			t.Fatal(err)
		}
		if gone.Kind != urllifecycle.ResolutionGone ||
			unknown.Kind != urllifecycle.ResolutionUnknown {
			t.Fatalf("gone/unknown drifted: %s/%s", gone.Kind, unknown.Kind)
		}
	})
}

func route(kind urllifecycle.ResourceKind, id, variant string) urllifecycle.RouteKey {
	return urllifecycle.RouteKey{
		Resource: urllifecycle.ResourceKey{Kind: kind, ID: id},
		Variant:  variant,
	}
}

func meta(id string) urllifecycle.MutationMeta {
	return urllifecycle.MutationMeta{
		CommandID: urllifecycle.CommandID("conformance-" + id),
		Reason:    "adapter conformance",
	}
}
