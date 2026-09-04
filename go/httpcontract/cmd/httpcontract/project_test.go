package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectGenerateAndCheck(t *testing.T) {
	directory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	t.Setenv("HTTP_CONTRACT_HELPER", "1")
	catalog := `{"schemaVersion":"errors.yueli.dev/catalog/v1","namespace":"demo","errors":[{"code":"demo.invalid","goName":"Invalid","status":400,"messageKey":"errors.demo.invalid"}]}`
	errors := `{"schemaVersion":"http.yueli.dev/operation-errors/v1","namespace":"demo","operations":{"POST /api/v1/widgets":["demo.invalid"]}}`
	project := `{"schemaVersion":"http.yueli.dev/project/v1","namespace":"demo","openapi":{"output":"openapi.json","producer":{"command":[` + quote(os.Args[0]) + `,"-test.run=TestProjectProducerHelper"],"outputEnv":"DEMO_OPENAPI_OUTPUT"}},"errorCatalog":"errors.json","operations":{"output":"operations.json","errorsFile":"operation-errors.json"},"generate":{"goOutput":"catalog_gen.go","goPackage":"demoerr","tsOutput":"failure.ts","tsType":"DemoFailure","i18nOutput":"i18n.json"},"legacyCatalog":{"output":"legacy.json","schemaVersion":"demo/errors/v1"}}`
	for name, data := range map[string]string{"errors.json": catalog, "operation-errors.json": errors, "project.json": project} {
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"-project", "project.json"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"openapi.json", "operations.json", "catalog_gen.go", "failure.ts", "i18n.json", "legacy.json"} {
		if _, err := os.Stat(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := run([]string{"-project", "project.json", "-check"}); err != nil {
		t.Fatal(err)
	}
}

func TestProjectProducerHelper(t *testing.T) {
	if os.Getenv("HTTP_CONTRACT_HELPER") != "1" {
		return
	}
	output := os.Getenv("DEMO_OPENAPI_OUTPUT")
	data := `{"paths":{"/api/v1/widgets":{"post":{"responses":{"201":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/CreateWidgetRes"}}}}}}}},"components":{"schemas":{"CreateWidgetRes":{"properties":{"widget":{"type":"object"}}}}}}`
	if err := os.WriteFile(filepath.Clean(output), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quote(value string) string {
	result := `"`
	for _, r := range value {
		if r == '\\' || r == '"' {
			result += `\`
		}
		result += string(r)
	}
	return result + `"`
}
