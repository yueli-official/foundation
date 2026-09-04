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

func TestParseProjectWithOperationErrorsFile(t *testing.T) {
	project, err := httpcontract.ParseProject([]byte(`{
      "schemaVersion":"http.yueli.dev/project/v1","namespace":"docs",
      "openapi":{"output":"openapi.json","producer":{"command":["go","run","./cmd/docs"],"outputEnv":"DOCS_OPENAPI_OUTPUT"}},
      "errorCatalog":"errors.json","operations":{"output":"operations.json","errorsFile":"operation-errors.json"},
      "generate":{"goOutput":"catalog_gen.go","goPackage":"docserr","tsOutput":"failure.ts","tsType":"DocsFailure","i18nOutput":"i18n.json"}
    }`))
	if err != nil {
		t.Fatal(err)
	}
	if project.Operations.ErrorsFile != "operation-errors.json" {
		t.Fatalf("%#v", project.Operations)
	}
}

func TestParseOperationErrors(t *testing.T) {
	declarations, err := httpcontract.ParseOperationErrors([]byte(`{"schemaVersion":"http.yueli.dev/operation-errors/v1","namespace":"docs","operations":{"POST /api/v1/docs":["docs.invalid_input"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(declarations.Operations) != 1 {
		t.Fatalf("%#v", declarations)
	}
	_, err = httpcontract.ParseOperationErrors([]byte(`{"schemaVersion":"http.yueli.dev/operation-errors/v1","namespace":"docs","operations":{"BROKEN":["docs.invalid_input"]}}`))
	if err == nil {
		t.Fatal("invalid route was accepted")
	}
}

func TestOperationErrorsFromOperations(t *testing.T) {
	operations, err := httpcontract.ParseOperations([]byte(validOperations))
	if err != nil {
		t.Fatal(err)
	}
	declarations := httpcontract.OperationErrorsFromOperations(operations)
	if len(declarations.Operations) != 1 {
		t.Fatalf("%#v", declarations)
	}
	encoded, err := httpcontract.EncodeOperationErrors(declarations)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := httpcontract.ParseOperationErrors(encoded); err != nil {
		t.Fatal(err)
	}
}
