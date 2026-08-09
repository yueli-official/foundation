// Package openapi exports the route-derived GoFrame OpenAPI document using
// explicit process-entry-point configuration.
package openapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
)

type ExportConfig struct {
	Server    *ghttp.Server
	Output    string
	Overwrite bool
	DirMode   os.FileMode
}

func Export(cfg ExportConfig) error {
	if cfg.Server == nil {
		return errors.New("OpenAPI export server is nil")
	}
	output := strings.TrimSpace(cfg.Output)
	if output == "" {
		return errors.New("OpenAPI export output is empty")
	}
	dirMode := cfg.DirMode
	if dirMode == 0 {
		dirMode = 0o755
	}
	if !cfg.Overwrite {
		if _, err := os.Stat(output); err == nil {
			return fmt.Errorf("OpenAPI export output already exists: %s", output)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	cfg.Server.SetAddr("127.0.0.1:0")
	cfg.Server.SetOpenApiPath("/api.json")
	cfg.Server.SetSwaggerPath("")
	cfg.Server.SetDumpRouterMap(false)
	if err := cfg.Server.Start(); err != nil {
		return fmt.Errorf("start OpenAPI export server: %w", err)
	}
	defer cfg.Server.Shutdown()
	document, err := marshalCanonicalJSON(cfg.Server.GetOpenApi())
	if err != nil {
		return fmt.Errorf("marshal OpenAPI: %w", err)
	}
	document = append(document, '\n')
	directory := filepath.Dir(output)
	if err := os.MkdirAll(directory, dirMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".openapi-*.json")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(document); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if cfg.Overwrite {
		if err := os.Remove(output); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(temporaryName, output)
}

// marshalCanonicalJSON normalizes custom JSON marshalers through ordinary
// string-keyed maps before indenting. GoFrame's OpenAPI schema types preserve
// insertion order for some objects, while those objects may be populated from
// Go maps. Normalizing here keeps generated contracts byte-for-byte stable.
func marshalCanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return json.MarshalIndent(normalized, "", "  ")
}
