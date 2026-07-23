package httpadapter_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/siteprofile"
	"github.com/yueli-official/foundation/go/siteprofile/httpadapter"
	"github.com/yueli-official/foundation/go/siteprofile/siteprofiletest"
)

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

func TestConditionalHTTPContract(t *testing.T) {
	clock := fixedClock{value: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)}
	module := siteprofile.NewMemory(siteprofile.MustCompileDefinition(siteprofile.DefaultDefinition()), clock)
	handler := httpadapter.MustNew(module, clock)

	body := encode(t, map[string]any{"profile": siteprofiletest.ValidProfile()})
	bootstrap := httptest.NewRequest(http.MethodPut, "/admin", bytes.NewReader(body))
	bootstrap.Header.Set("If-None-Match", "*")
	bootstrapResult := httptest.NewRecorder()
	handler.Admin(bootstrapResult, bootstrap)
	if bootstrapResult.Code != http.StatusOK || bootstrapResult.Header().Get("ETag") == "" {
		t.Fatalf("bootstrap status=%d body=%s", bootstrapResult.Code, bootstrapResult.Body.String())
	}
	etag := bootstrapResult.Header().Get("ETag")

	public := httptest.NewRequest(http.MethodGet, "/public", nil)
	public.Header.Set("If-None-Match", etag)
	publicResult := httptest.NewRecorder()
	handler.Public(publicResult, public)
	if publicResult.Code != http.StatusNotModified || publicResult.Header().Get("Cache-Control") != "public, no-cache" {
		t.Fatalf("public status=%d headers=%v", publicResult.Code, publicResult.Header())
	}

	missingPrecondition := httptest.NewRequest(http.MethodPut, "/admin", bytes.NewReader(body))
	missingResult := httptest.NewRecorder()
	handler.Admin(missingResult, missingPrecondition)
	if missingResult.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing precondition status=%d", missingResult.Code)
	}

	stale := httptest.NewRequest(http.MethodPut, "/admin", bytes.NewReader(body))
	stale.Header.Set("If-Match", `"stale"`)
	staleResult := httptest.NewRecorder()
	handler.Admin(staleResult, stale)
	if staleResult.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale status=%d", staleResult.Code)
	}

	update := httptest.NewRequest(http.MethodPut, "/admin", bytes.NewReader(body))
	update.Header.Set("If-Match", etag)
	updateResult := httptest.NewRecorder()
	handler.Admin(updateResult, update)
	if updateResult.Code != http.StatusOK || updateResult.Header().Get("ETag") != etag {
		t.Fatalf("no-op update status=%d etag=%q body=%s", updateResult.Code, updateResult.Header().Get("ETag"), updateResult.Body.String())
	}
}

func TestValidationAndSchemaHTTP(t *testing.T) {
	clock := fixedClock{value: time.Now()}
	module := siteprofile.NewMemory(siteprofile.MustCompileDefinition(siteprofile.DefaultDefinition()), clock)
	handler := httpadapter.MustNew(module, clock)
	invalid := siteprofiletest.ValidProfile()
	invalid.Identity.Name = ""
	request := httptest.NewRequest(http.MethodPut, "/admin", bytes.NewReader(encode(t, map[string]any{"profile": invalid})))
	request.Header.Set("If-None-Match", "*")
	response := httptest.NewRecorder()
	handler.Admin(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("validation status=%d body=%s", response.Code, response.Body.String())
	}
	schema := httptest.NewRequest(http.MethodGet, "/schema", nil)
	schemaResponse := httptest.NewRecorder()
	handler.Schema(schemaResponse, schema)
	if schemaResponse.Code != http.StatusOK || schemaResponse.Header().Get("ETag") == "" {
		t.Fatalf("schema status=%d headers=%v", schemaResponse.Code, schemaResponse.Header())
	}
}

func encode(t *testing.T, value any) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(value); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
