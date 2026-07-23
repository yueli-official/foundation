// Package discoverytest provides public Discovery Adapter conformance suites.
package discoverytest

import (
	"context"
	"errors"
	"testing"

	"github.com/yueli-official/foundation/go/discovery"
)

type TargetHarness interface {
	Target() discovery.PublishTarget
	Artifact(name string) ([]byte, bool)
}

type TargetFactory func(*testing.T) TargetHarness

// RunPublishTarget verifies staging, commit and failure rollback through the
// public Discovery interface.
func RunPublishTarget(t *testing.T, factory TargetFactory) {
	t.Helper()
	t.Run("commit exposes complete publication", func(t *testing.T) {
		harness := factory(t)
		module := testModule(t)
		_, _, err := module.Publish(context.Background(), discovery.PublicationPlan{
			Robots: &discovery.RobotsPlan{},
		}, nil, harness.Target())
		if err != nil {
			t.Fatal(err)
		}
		value, ok := harness.Artifact("robots.txt")
		if !ok || len(value) == 0 {
			t.Fatal("committed artifact is not visible")
		}
	})
	t.Run("source failure preserves previous publication", func(t *testing.T) {
		harness := factory(t)
		module := testModule(t)
		_, _, err := module.Publish(context.Background(), discovery.PublicationPlan{
			Robots: &discovery.RobotsPlan{},
		}, nil, harness.Target())
		if err != nil {
			t.Fatal(err)
		}
		before, ok := harness.Artifact("robots.txt")
		if !ok {
			t.Fatal("initial artifact is missing")
		}
		_, _, err = module.Publish(context.Background(), discovery.PublicationPlan{
			Sitemap: &discovery.SitemapPlan{Source: "broken"},
		}, discovery.Sources{"broken": errorSource{}}, harness.Target())
		if !discovery.IsKind(err, discovery.ErrorSource) {
			t.Fatalf("expected source failure, got %v", err)
		}
		after, ok := harness.Artifact("robots.txt")
		if !ok || string(after) != string(before) {
			t.Fatal("aborted publication changed visible artifacts")
		}
	})
}

func testModule(t *testing.T) *discovery.Module {
	t.Helper()
	module, err := discovery.Compile(discovery.Definition{
		ContractVersion: discovery.ContractVersion,
		Site: discovery.SiteProfile{
			Origin: "https://example.test", Name: "Conformance",
			DefaultLocale: "en",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return module
}

type errorSource struct{}

func (errorSource) Next(context.Context, discovery.Cursor, int) (discovery.Batch, error) {
	return discovery.Batch{}, errors.New("adapter unavailable")
}
