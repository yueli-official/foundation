package capability

import (
	"bytes"
	"encoding/json"
	"testing"
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
