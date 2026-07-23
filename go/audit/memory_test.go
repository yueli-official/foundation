package audit_test

import (
	"testing"

	"github.com/yueli-official/foundation/go/audit"
	"github.com/yueli-official/foundation/go/audit/audittest"
)

func TestMemoryConformance(t *testing.T) {
	audittest.Run(t, func(t *testing.T, catalog *audit.Catalog, clock audit.Clock) audit.Module {
		t.Helper()
		module, err := audit.NewMemory(catalog, audit.MemoryOptions{
			Clock: clock,
			Source: audit.Source{
				Service: "docs", Module: "catalog", Instance: "docs-main", Version: "test",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return module
	})
}
