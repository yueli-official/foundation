package traffic_test

import (
	"testing"

	"github.com/yueli-official/foundation/go/traffic"
	"github.com/yueli-official/foundation/go/traffic/traffictest"
)

func TestMemoryConformance(t *testing.T) {
	traffictest.Run(t, func(t *testing.T, catalog *traffic.Catalog, clock *traffictest.Clock) traffic.Module {
		t.Helper()
		module, err := traffic.NewMemory(catalog, traffic.MemoryOptions{
			Clock: clock.Now, Secret: []byte("memory-conformance-secret-32-bytes"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return module
	})
}
