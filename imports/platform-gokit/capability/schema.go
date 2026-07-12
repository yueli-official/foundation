package capability

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed service-capability-manifest.schema.json
var schemaJSON []byte

// Schema returns an isolated copy of the canonical JSON Schema artifact.
func Schema() []byte {
	return append([]byte(nil), schemaJSON...)
}

func validateEmbeddedSchema() error {
	var document struct {
		Schema string `json:"$schema"`
		ID     string `json:"$id"`
		Type   string `json:"type"`
	}
	if err := json.Unmarshal(schemaJSON, &document); err != nil {
		return fmt.Errorf("decode embedded capability schema: %w", err)
	}
	if document.Schema != "https://json-schema.org/draft/2020-12/schema" || document.ID != "https://platform.yueli.dev/schemas/service-capability-manifest.v1.schema.json" || document.Type != "object" {
		return fmt.Errorf("embedded capability schema identity is invalid")
	}
	return nil
}
