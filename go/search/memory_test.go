package search_test

import (
	"testing"

	"github.com/yueli-official/foundation/go/search"
	"github.com/yueli-official/foundation/go/search/searchtest"
)

func TestMemoryConformance(t *testing.T) {
	searchtest.Run(t, func(_ *testing.T, catalog *search.Catalog) search.Module {
		return search.NewMemory(catalog)
	})
}
