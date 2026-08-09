package openapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

type pingRequest struct {
	g.Meta `path:"/ping" method:"get" summary:"Ping"`
}
type pingResponse struct {
	Status string `json:"status"`
}
type pingController struct{}

func (pingController) Ping(context.Context, *pingRequest) (*pingResponse, error) {
	return &pingResponse{Status: "ok"}, nil
}

func TestExportWritesRouteDocumentAndRequiresOverwrite(t *testing.T) {
	output := filepath.Join(t.TempDir(), "nested", "service.json")
	server := g.Server(t.Name())
	server.Group("/", func(group *ghttp.RouterGroup) { group.Bind(pingController{}) })
	if err := Export(ExportConfig{Server: server, Output: output}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	paths, _ := document["paths"].(map[string]any)
	if _, ok := paths["/ping"]; !ok {
		t.Fatalf("paths = %v", paths)
	}
	if err := Export(ExportConfig{Server: g.Server(t.Name() + "Existing"), Output: output}); err == nil {
		t.Fatal("existing output overwritten implicitly")
	}
}

func TestExportRejectsMissingInputs(t *testing.T) {
	if err := Export(ExportConfig{}); err == nil {
		t.Fatal("nil server accepted")
	}
	if err := Export(ExportConfig{Server: g.Server(t.Name())}); err == nil {
		t.Fatal("empty output accepted")
	}
}

type insertionOrderedJSON struct{}

func (insertionOrderedJSON) MarshalJSON() ([]byte, error) {
	return []byte(`{"z":1,"a":{"y":2,"b":3}}`), nil
}

func TestMarshalCanonicalJSONSortsCustomMarshalerObjects(t *testing.T) {
	got, err := marshalCanonicalJSON(insertionOrderedJSON{})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": {\n    \"b\": 3,\n    \"y\": 2\n  },\n  \"z\": 1\n}"
	if string(got) != want {
		t.Fatalf("canonical JSON = %s", got)
	}
}
