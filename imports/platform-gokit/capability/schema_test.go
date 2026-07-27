package capability

import (
	"bytes"
	"encoding/json"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestEmbeddedSchemaIsCanonicalAndIsolated(t *testing.T) {
	if err := validateEmbeddedSchema(); err != nil {
		t.Fatal(err)
	}
	first := Schema()
	var document any
	if err := json.Unmarshal(first, &document); err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || first[len(first)-1] != '\n' {
		t.Fatal("embedded capability schema must end with a newline")
	}
	first[0] = 'x'
	if bytes.Equal(first, Schema()) {
		t.Fatal("Schema returned shared mutable storage")
	}
}

func TestSchemaValidatesManifestExamples(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	var schemaDocument any
	if err := json.Unmarshal(Schema(), &schemaDocument); err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource(SchemaID, schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(SchemaID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewSnapshot(validManifest())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(snapshot.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	decode := func(t *testing.T) map[string]any {
		t.Helper()
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		return document
	}
	if err := schema.Validate(decode(t)); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing nested field", func(document map[string]any) { delete(document["service"].(map[string]any), "version") }},
		{"unknown state", func(document map[string]any) {
			document["capabilities"].([]any)[0].(map[string]any)["health"] = "maybe"
		}},
		{"secret value", func(document map[string]any) {
			document["providers"].([]any)[0].(map[string]any)["requiredConfig"].([]any)[1].(map[string]any)["value"] = "secret"
		}},
		{"unsafe link", func(document map[string]any) {
			document["links"].([]any)[0].(map[string]any)["href"] = "https://example.com/admin"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := decode(t)
			test.mutate(document)
			if err := schema.Validate(document); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}
