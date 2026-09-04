package httpcontract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yueli-official/foundation/go/httpcontract"
)

const validCatalog = `{
  "schemaVersion":"errors.yueli.dev/catalog/v1",
  "namespace":"docs",
  "errors":[{
    "code":"docs.import.compression_unsupported",
    "goName":"ImportCompressionUnsupported",
    "status":400,
    "messageKey":"docs.errors.import.compression_unsupported",
    "recoveryKey":"docs.actions.repack_archive",
    "params":{"method":{"type":"string","required":true,"maxLength":32}},
    "violations":"forbidden"
  }]
}`

const validOperations = `{
  "schemaVersion":"http.yueli.dev/operations/v1",
  "namespace":"docs",
  "operations":[
    {"id":"docs.collections.list","method":"GET","path":"/api/v1/collections","success":{"status":200,"kind":"page","schemaRef":"#/components/schemas/Collection"},"errors":["common.internal"]},
    {"id":"docs.collections.create","method":"POST","path":"/api/v1/collections","success":{"status":201,"kind":"resource","schemaRef":"#/components/schemas/Collection"}},
    {"id":"docs.collections.delete","method":"DELETE","path":"/api/v1/collections/{id}","success":{"status":204,"kind":"empty"}}
  ]
}`

func TestParseErrorCatalog(t *testing.T) {
	catalog, err := httpcontract.ParseErrorCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Namespace != "docs" || len(catalog.Errors) != 1 {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestParseErrorCatalogRejectsSemanticDrift(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want string
	}{
		{"foreign namespace", `docs.import.compression_unsupported`, `asset.import.compression_unsupported`, "must belong to namespace"},
		{"duplicate", `}]`, `},{"code":"docs.import.compression_unsupported","status":400,"messageKey":"docs.errors.duplicate"}]`, "duplicate error code"},
		{"invalid parameter constraint", `"maxLength":32`, `"maxLength":32,"maxItems":2`, "maxItems is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := httpcontract.ParseErrorCatalog([]byte(strings.Replace(validCatalog, test.from, test.to, 1)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseOperations(t *testing.T) {
	manifest, err := httpcontract.ParseOperations([]byte(validOperations))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Operations) != 3 {
		t.Fatalf("operations = %#v", manifest.Operations)
	}
}

func TestParseOperationsKeepsOAuthWireErrorsProtocolNative(t *testing.T) {
	manifest, err := httpcontract.ParseOperations([]byte(`{
      "schemaVersion":"http.yueli.dev/operations/v1",
      "namespace":"identity",
      "operations":[{
        "id":"identity.oauth.token",
        "method":"POST",
        "path":"/oauth2/token",
        "failureProtocol":"oauth",
        "success":{"status":200,"kind":"resource","schemaRef":"#/components/schemas/TokenResponse"},
        "errors":["invalid_request","invalid_client","invalid_grant","temporarily_unavailable"]
      }]
    }`))
	if err != nil {
		t.Fatal(err)
	}
	if err := httpcontract.VerifyReferences(httpcontract.ErrorCatalog{Namespace: "identity"}, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestParseOperationsRejectsWrongSuccessShape(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want string
	}{
		{`"status":201,"kind":"resource"`, `"status":200,"kind":"operation"`, "operation status must be 202"},
		{`"status":204,"kind":"empty"`, `"status":204,"kind":"empty","schemaRef":"#/components/schemas/Empty"`, "schemaRef is not allowed"},
		{`"id":"docs.collections.list"`, `"id":"asset.collections.list"`, "must belong to namespace"},
	}
	for _, test := range tests {
		_, err := httpcontract.ParseOperations([]byte(strings.Replace(validOperations, test.from, test.to, 1)))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("error = %v, want %q", err, test.want)
		}
	}
}

func TestVerifyReferences(t *testing.T) {
	catalog, err := httpcontract.ParseErrorCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	operations, err := httpcontract.ParseOperations([]byte(strings.Replace(
		validOperations,
		`"errors":["common.internal"]`,
		`"errors":["common.internal","docs.import.compression_unsupported"]`,
		1,
	)))
	if err != nil {
		t.Fatal(err)
	}
	if err := httpcontract.VerifyReferences(catalog, operations); err != nil {
		t.Fatal(err)
	}
	operations.Operations[0].Errors = append(operations.Operations[0].Errors, "docs.missing")
	if err := httpcontract.VerifyReferences(catalog, operations); err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateGoCatalog(t *testing.T) {
	catalog, err := httpcontract.ParseErrorCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	generated, err := httpcontract.GenerateGo(catalog, "docserr")
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	for _, expected := range []string{
		`CodeImportCompressionUnsupported = "docs.import.compression_unsupported"`,
		`descriptor(CodeImportCompressionUnsupported, 400)`,
		`func DescriptorForCode(code string)`,
		`func Catalog() []CatalogEntry`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated catalog missing %q:\n%s", expected, text)
		}
	}
}

func TestGenerateTypeScriptCatalogAndI18nInventory(t *testing.T) {
	catalog, err := httpcontract.ParseErrorCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	typescript, err := httpcontract.GenerateTypeScript(catalog, "DocsFailure")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`export type DocsFailureCode =`,
		`readonly code: "docs.import.compression_unsupported"`,
		`readonly method: string`,
		`readonly violations?: never`,
		`export const docsFailurePresentation =`,
		`recoveryKey: "docs.actions.repack_archive"`,
	} {
		if !strings.Contains(string(typescript), expected) {
			t.Fatalf("generated TypeScript missing %q:\n%s", expected, typescript)
		}
	}
	inventory, err := httpcontract.GenerateI18nInventory(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"messageKey": "docs.errors.import.compression_unsupported"`, `"recoveryKey": "docs.actions.repack_archive"`, `"method"`} {
		if !strings.Contains(string(inventory), expected) {
			t.Fatalf("generated inventory missing %q:\n%s", expected, inventory)
		}
	}
}

func TestCompatibilityDiffClassifiesChanges(t *testing.T) {
	before, err := httpcontract.ParseErrorCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	after := before
	after.Errors = append([]httpcontract.ErrorDefinition(nil), before.Errors...)
	after.Errors[0].Status = 422
	after.Errors = append(after.Errors, httpcontract.ErrorDefinition{Code: "docs.new_error", Status: 400, MessageKey: "docs.errors.new_error"})
	report := httpcontract.DiffErrorCatalogs(before, after)
	if !report.HasBreaking() || len(report.Changes) != 2 {
		t.Fatalf("report = %#v", report)
	}
	if report.Changes[0].Severity != httpcontract.ChangeBreaking || report.Changes[1].Severity != httpcontract.ChangeAdditive {
		t.Fatalf("changes = %#v", report.Changes)
	}

	oldOperations, err := httpcontract.ParseOperations([]byte(validOperations))
	if err != nil {
		t.Fatal(err)
	}
	newOperations := oldOperations
	newOperations.Operations = append([]httpcontract.Operation(nil), oldOperations.Operations...)
	newOperations.Operations[0].Success.Kind = "cursorPage"
	operationReport := httpcontract.DiffOperations(oldOperations, newOperations)
	if !operationReport.HasBreaking() || len(operationReport.Changes) != 1 {
		t.Fatalf("operation report = %#v", operationReport)
	}
}

func TestJSONSchemasAcceptCanonicalExamples(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "http-result")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skip("repository contracts are not present in the published Go module")
	}
	for _, test := range []struct {
		name     string
		schema   string
		instance string
	}{
		{"catalog", "error-catalog.v1.schema.json", validCatalog},
		{"operations", "operations.v1.schema.json", validOperations},
	} {
		t.Run(test.name, func(t *testing.T) {
			compiler := jsonschema.NewCompiler()
			schema, err := compiler.Compile(filepath.Join(root, test.schema))
			if err != nil {
				t.Fatal(err)
			}
			var document any
			if err := json.Unmarshal([]byte(test.instance), &document); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(document); err != nil {
				t.Fatal(err)
			}
		})
	}
}
