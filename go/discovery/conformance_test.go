package discovery_test

import (
	"testing"

	"github.com/yueli-official/foundation/go/discovery"
	"github.com/yueli-official/foundation/go/discovery/discoverytest"
)

type memoryHarness struct {
	target *discovery.MemoryTarget
}

func (harness memoryHarness) Target() discovery.PublishTarget {
	return harness.target
}

func (harness memoryHarness) Artifact(name string) ([]byte, bool) {
	value, ok := harness.target.Snapshot().Artifacts[name]
	return value, ok
}

func TestMemoryPublishTargetConformance(t *testing.T) {
	discoverytest.RunPublishTarget(t, func(*testing.T) discoverytest.TargetHarness {
		return memoryHarness{target: &discovery.MemoryTarget{}}
	})
}
