package httpcontract_test

import (
	"strings"
	"testing"

	"github.com/yueli-official/foundation/go/httpcontract"
)

func TestOperationsFromOpenAPI(t *testing.T) {
	openAPI := []byte(`{"paths":{"/api/v1/widgets":{"get":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/ListWidgetsRes"}}}}}},"post":{"responses":{"201":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CreateWidgetRes"}}}}}}}},"components":{"schemas":{"ListWidgetsRes":{"properties":{"items":{"type":"array"}}},"CreateWidgetRes":{"properties":{"widget":{"type":"object"}}}}}}`)
	operations, err := httpcontract.OperationsFromOpenAPI(openAPI, "widgets", map[string][]string{"POST /api/v1/widgets": {"widgets.invalid"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(operations.Operations) != 2 {
		t.Fatalf("operations=%d", len(operations.Operations))
	}
	if operations.Operations[0].Success.Kind != "collection" || operations.Operations[1].Success.Status != 201 {
		t.Fatalf("unexpected projection: %#v", operations.Operations)
	}
}

func TestOperationsFromOpenAPIRejectsStaleErrorRoute(t *testing.T) {
	openAPI := []byte(`{"paths":{"/ready":{"get":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Ready"}}}}}}}},"components":{"schemas":{"Ready":{"properties":{}}}}}`)
	_, err := httpcontract.OperationsFromOpenAPI(openAPI, "docs", map[string][]string{"POST /removed": {"docs.invalid_input"}})
	if err == nil || !strings.Contains(err.Error(), "stale route") {
		t.Fatalf("error=%v", err)
	}
}

func TestGenerateLegacyCatalog(t *testing.T) {
	catalog, err := httpcontract.ParseErrorCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	data, err := httpcontract.GenerateLegacyCatalog(catalog, "example/errors/v1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schemaVersion": "example/errors/v1"`) {
		t.Fatalf("%s", data)
	}
}
