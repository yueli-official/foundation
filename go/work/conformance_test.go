package work_test

import (
	"testing"

	"github.com/yueli-official/foundation/go/work"
	"github.com/yueli-official/foundation/go/work/worktest"
)

func TestMemoryConformance(t *testing.T) {
	worktest.Run(t, func(t *testing.T, catalog *work.Catalog, clock *worktest.Clock) work.Backend {
		t.Helper()
		memory, err := work.NewMemory(catalog, work.MemoryOptions{Clock: clock.Now})
		if err != nil {
			t.Fatal(err)
		}
		return memory
	})
}
