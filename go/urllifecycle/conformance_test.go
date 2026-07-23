package urllifecycle_test

import (
	"testing"

	"github.com/yueli-official/foundation/go/urllifecycle"
	"github.com/yueli-official/foundation/go/urllifecycle/urllifecycletest"
)

func TestMemoryAdapterConformance(t *testing.T) {
	urllifecycletest.Run(t, func(t *testing.T, catalog *urllifecycle.Catalog) urllifecycle.Module {
		t.Helper()
		module, err := urllifecycle.NewMemory(catalog, urllifecycle.MemoryOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return module
	})
}
