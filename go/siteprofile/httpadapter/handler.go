package httpadapter

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/yueli-official/foundation/go/siteprofile"
)

type Handler struct {
	module siteprofile.Module
	clock  siteprofile.Clock
}

func New(module siteprofile.Module, clock siteprofile.Clock) (*Handler, error) {
	if module == nil {
		return nil, errors.New("siteprofile/httpadapter: module is required")
	}
	if clock == nil {
		clock = siteprofile.SystemClock{}
	}
	return &Handler{module: module, clock: clock}, nil
}

func MustNew(module siteprofile.Module, clock siteprofile.Clock) *Handler {
	handler, err := New(module, clock)
	if err != nil {
		panic(err)
	}
	return handler
}

func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}
	projection, err := h.module.PublicAt(r.Context(), h.clock.Now())
	if err != nil {
		writeModuleError(w, err)
		return
	}
	w.Header().Set("ETag", projection.Snapshot.ETag)
	w.Header().Set("Cache-Control", "public, no-cache")
	if etagMatches(r.Header.Get("If-None-Match"), projection.Snapshot.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, r, http.StatusOK, projection)
}

func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		snapshot, err := h.module.Get(r.Context())
		if err != nil {
			writeModuleError(w, err)
			return
		}
		w.Header().Set("ETag", snapshot.ETag)
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, r, http.StatusOK, snapshot)
	case http.MethodPut:
		h.replace(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodHead, http.MethodPut)
	}
}

func (h *Handler) Schema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}
	schema := h.module.Schema()
	etag := `"` + string(schema.Digest) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, no-cache")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, r, http.StatusOK, schema)
}

func (h *Handler) replace(w http.ResponseWriter, r *http.Request) {
	expected, ok := h.expectedRevision(w, r)
	if !ok {
		return
	}
	var body struct {
		Profile siteprofile.Profile `json:"profile"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "request body is invalid")
		return
	}
	result, err := h.module.Replace(r.Context(), siteprofile.ReplaceCommand{
		ExpectedRevision: expected,
		Profile:          body.Profile,
	})
	if err != nil {
		writeModuleError(w, err)
		return
	}
	w.Header().Set("ETag", result.Snapshot.ETag)
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, r, http.StatusOK, result)
}

func (h *Handler) expectedRevision(w http.ResponseWriter, r *http.Request) (siteprofile.Revision, bool) {
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == "*" {
		return 0, true
	}
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if ifMatch == "" {
		writeProblem(w, http.StatusPreconditionRequired, "precondition_required", "If-Match or If-None-Match: * is required")
		return 0, false
	}
	current, err := h.module.Get(r.Context())
	if err != nil {
		writeModuleError(w, err)
		return 0, false
	}
	if !strongETagMatches(ifMatch, current.ETag) {
		writeProblem(w, http.StatusPreconditionFailed, "revision_conflict", "If-Match does not match the current profile")
		return 0, false
	}
	return current.Revision, true
}

func writeModuleError(w http.ResponseWriter, err error) {
	var validation *siteprofile.ValidationError
	var conflict *siteprofile.RevisionConflictError
	switch {
	case errors.Is(err, siteprofile.ErrNotInitialized):
		writeProblem(w, http.StatusNotFound, "not_initialized", err.Error())
	case errors.Is(err, siteprofile.ErrCorruptState):
		writeProblem(w, http.StatusServiceUnavailable, "corrupt_state", "site profile is unavailable")
	case errors.As(err, &validation):
		writeJSON(w, nil, http.StatusUnprocessableEntity, map[string]any{
			"code": "validation_failed", "message": validation.Error(), "diagnostics": validation.Diagnostics,
		})
	case errors.As(err, &conflict):
		writeProblem(w, http.StatusPreconditionFailed, "revision_conflict", conflict.Error())
	default:
		writeProblem(w, http.StatusServiceUnavailable, "unavailable", "site profile is unavailable")
	}
}

func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, nil, status, map[string]string{"code": code, "message": message})
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if r != nil && r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "method is not allowed")
}

func etagMatches(header, target string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == target {
			return true
		}
	}
	return false
}

func strongETagMatches(header, target string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == target {
			return true
		}
	}
	return false
}
