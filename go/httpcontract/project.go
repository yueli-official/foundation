package httpcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const ProjectSchemaVersion = "http.yueli.dev/project/v1"
const OperationErrorsSchemaVersion = "http.yueli.dev/operation-errors/v1"

type Project struct {
	SchemaVersion     string                `json:"schemaVersion"`
	Namespace         string                `json:"namespace"`
	OpenAPI           ProjectOpenAPI        `json:"openapi"`
	ErrorCatalog      string                `json:"errorCatalog"`
	Operations        ProjectOperations     `json:"operations"`
	Generate          ProjectGenerate       `json:"generate"`
	LegacyCatalog     *ProjectLegacyCatalog `json:"legacyCatalog,omitempty"`
	AllowUnusedErrors []string              `json:"allowUnusedErrors,omitempty"`
}

type ProjectOpenAPI struct {
	Output   string                 `json:"output"`
	Producer ProjectOpenAPIProducer `json:"producer"`
}
type ProjectOpenAPIProducer struct {
	Command   []string `json:"command"`
	OutputEnv string   `json:"outputEnv"`
}
type ProjectOperations struct {
	Output     string              `json:"output"`
	ErrorsFile string              `json:"errorsFile,omitempty"`
	Errors     map[string][]string `json:"errors,omitempty"`
}
type ProjectGenerate struct {
	GoOutput   string `json:"goOutput"`
	GoPackage  string `json:"goPackage"`
	TSOutput   string `json:"tsOutput"`
	TSType     string `json:"tsType"`
	I18nOutput string `json:"i18nOutput"`
}
type ProjectLegacyCatalog struct {
	Output        string `json:"output"`
	SchemaVersion string `json:"schemaVersion"`
}

func ParseProject(data []byte) (Project, error) {
	var project Project
	if err := decodeStrict(data, &project); err != nil {
		return Project{}, fmt.Errorf("httpcontract: decode project: %w", err)
	}
	if err := project.Validate(); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (project Project) Validate() error {
	if project.SchemaVersion != ProjectSchemaVersion {
		return fmt.Errorf("httpcontract: unsupported project schemaVersion %q", project.SchemaVersion)
	}
	if !namespacePattern.MatchString(project.Namespace) {
		return errors.New("httpcontract: project namespace is invalid")
	}
	for name, value := range map[string]string{"openapi.output": project.OpenAPI.Output, "errorCatalog": project.ErrorCatalog, "operations.output": project.Operations.Output, "generate.goOutput": project.Generate.GoOutput, "generate.tsOutput": project.Generate.TSOutput, "generate.i18nOutput": project.Generate.I18nOutput} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("httpcontract: project %s is required", name)
		}
	}
	if len(project.OpenAPI.Producer.Command) == 0 || strings.TrimSpace(project.OpenAPI.Producer.Command[0]) == "" {
		return errors.New("httpcontract: project openapi.producer.command is required")
	}
	if (project.Operations.ErrorsFile == "") == (project.Operations.Errors == nil) {
		return errors.New("httpcontract: project operations requires exactly one of errorsFile or errors")
	}
	if strings.TrimSpace(project.OpenAPI.Producer.OutputEnv) == "" {
		return errors.New("httpcontract: project openapi.producer.outputEnv is required")
	}
	if !regexpTypeName.MatchString(project.Generate.TSType) {
		return errors.New("httpcontract: project generate.tsType is invalid")
	}
	if !goPackagePattern.MatchString(project.Generate.GoPackage) {
		return errors.New("httpcontract: project generate.goPackage is invalid")
	}
	if project.LegacyCatalog != nil && (project.LegacyCatalog.Output == "" || project.LegacyCatalog.SchemaVersion == "") {
		return errors.New("httpcontract: project legacyCatalog output and schemaVersion are required")
	}
	seenUnused := make(map[string]struct{}, len(project.AllowUnusedErrors))
	for _, code := range project.AllowUnusedErrors {
		if !codePattern.MatchString(code) || !strings.HasPrefix(code, project.Namespace+".") {
			return fmt.Errorf("httpcontract: project allowUnusedErrors contains invalid code %q", code)
		}
		if _, ok := seenUnused[code]; ok {
			return fmt.Errorf("httpcontract: project allowUnusedErrors contains duplicate code %q", code)
		}
		seenUnused[code] = struct{}{}
	}
	return nil
}

type OperationErrors struct {
	SchemaVersion string                       `json:"schemaVersion"`
	Namespace     string                       `json:"namespace"`
	Operations    map[string][]string          `json:"operations"`
	IDs           map[string]string            `json:"ids,omitempty"`
	Overrides     map[string]OperationOverride `json:"overrides,omitempty"`
}
type OperationOverride struct {
	Success             *Success  `json:"success,omitempty"`
	FailureProtocol     string    `json:"failureProtocol,omitempty"`
	AdditionalSuccesses []Success `json:"additionalSuccesses,omitempty"`
}

func ParseOperationErrors(data []byte) (OperationErrors, error) {
	var result OperationErrors
	if err := decodeStrict(data, &result); err != nil {
		return OperationErrors{}, fmt.Errorf("httpcontract: decode operation errors: %w", err)
	}
	if result.SchemaVersion != OperationErrorsSchemaVersion {
		return OperationErrors{}, fmt.Errorf("httpcontract: unsupported operation errors schemaVersion %q", result.SchemaVersion)
	}
	if !namespacePattern.MatchString(result.Namespace) {
		return OperationErrors{}, errors.New("httpcontract: operation errors namespace is invalid")
	}
	if result.Operations == nil {
		return OperationErrors{}, errors.New("httpcontract: operation errors requires operations")
	}
	for route, codes := range result.Operations {
		parts := strings.SplitN(route, " ", 2)
		if len(parts) != 2 || !validMethod(parts[0]) || !strings.HasPrefix(parts[1], "/") {
			return OperationErrors{}, fmt.Errorf("httpcontract: operation errors route %q is invalid", route)
		}
		seen := make(map[string]struct{}, len(codes))
		for _, code := range codes {
			if !codePattern.MatchString(code) {
				return OperationErrors{}, fmt.Errorf("httpcontract: operation errors route %q contains invalid code %q", route, code)
			}
			if _, ok := seen[code]; ok {
				return OperationErrors{}, fmt.Errorf("httpcontract: operation errors route %q contains duplicate code %q", route, code)
			}
			seen[code] = struct{}{}
		}
	}
	for route, id := range result.IDs {
		if _, ok := result.Operations[route]; !ok {
			return OperationErrors{}, fmt.Errorf("httpcontract: operation errors id route %q has no operation declaration", route)
		}
		if !operationIDPattern.MatchString(id) || !strings.HasPrefix(id, result.Namespace+".") {
			return OperationErrors{}, fmt.Errorf("httpcontract: operation errors route %q has invalid id %q", route, id)
		}
	}
	for route, override := range result.Overrides {
		if _, ok := result.Operations[route]; !ok {
			return OperationErrors{}, fmt.Errorf("httpcontract: operation errors override route %q has no operation declaration", route)
		}
		if override.Success != nil {
			if err := override.Success.validate("problem"); err != nil {
				return OperationErrors{}, fmt.Errorf("httpcontract: operation errors override route %q: %w", route, err)
			}
		}
	}
	return result, nil
}

func OperationErrorsFromOperations(operations Operations) OperationErrors {
	result := OperationErrors{SchemaVersion: OperationErrorsSchemaVersion, Namespace: operations.Namespace, Operations: make(map[string][]string), IDs: make(map[string]string), Overrides: make(map[string]OperationOverride)}
	for _, operation := range operations.Operations {
		key := operation.Method + " " + operation.Path
		if len(operation.Errors) > 0 {
			result.Operations[key] = append([]string(nil), operation.Errors...)
			result.IDs[key] = operation.ID
		}
		if operation.Success.Kind == "binary" || operation.Success.Kind == "redirect" || operation.FailureProtocol != "" || len(operation.AdditionalSuccesses) > 0 {
			success := operation.Success
			result.Overrides[key] = OperationOverride{Success: &success, FailureProtocol: operation.FailureProtocol, AdditionalSuccesses: append([]Success(nil), operation.AdditionalSuccesses...)}
		}
	}
	return result
}

func EncodeOperationErrors(declarations OperationErrors) ([]byte, error) {
	data, err := json.MarshalIndent(declarations, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

type openAPIDocument struct {
	Paths      map[string]map[string]openAPIOperation `json:"paths"`
	Components struct {
		Schemas map[string]openAPISchema `json:"schemas"`
	} `json:"components"`
}
type openAPIOperation struct {
	Responses map[string]openAPIResponse `json:"responses"`
}
type openAPIResponse struct {
	Content map[string]struct {
		Schema struct {
			Ref string `json:"$ref"`
		} `json:"schema"`
	} `json:"content"`
}
type openAPISchema struct {
	Properties map[string]json.RawMessage `json:"properties"`
}

func OperationsFromOpenAPI(data []byte, namespace string, operationErrors map[string][]string) (Operations, error) {
	return OperationsFromOpenAPIWithIDs(data, namespace, operationErrors, nil)
}

func OperationsFromOpenAPIWithIDs(data []byte, namespace string, operationErrors map[string][]string, operationIDs map[string]string, overrides ...map[string]OperationOverride) (Operations, error) {
	var document openAPIDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return Operations{}, fmt.Errorf("httpcontract: decode OpenAPI: %w", err)
	}
	manifest := Operations{SchemaVersion: OperationsSchemaVersion, Namespace: namespace}
	routes := make(map[string]struct{})
	var operationOverrides map[string]OperationOverride
	if len(overrides) > 0 {
		operationOverrides = overrides[0]
	}
	for path, methods := range document.Paths {
		for method, operation := range methods {
			status, response, ok := projectSuccessResponse(operation.Responses)
			if !ok {
				continue
			}
			ref := projectSchemaRef(response)
			if status == 204 {
				ref = ""
			}
			key := strings.ToUpper(method) + " " + path
			routes[key] = struct{}{}
			id := operationIDs[key]
			if id == "" {
				id = projectOperationID(namespace, method, path)
			}
			item := Operation{ID: id, Method: strings.ToUpper(method), Path: path, Success: Success{Status: status, Kind: projectResponseKind(status, ref, document.Components.Schemas), SchemaRef: ref}, Errors: operationErrors[key]}
			if override, ok := operationOverrides[key]; ok {
				if override.Success != nil {
					item.Success = *override.Success
				}
				item.FailureProtocol = override.FailureProtocol
				item.AdditionalSuccesses = append([]Success(nil), override.AdditionalSuccesses...)
			}
			manifest.Operations = append(manifest.Operations, item)
		}
	}
	for route := range operationErrors {
		if _, ok := routes[route]; !ok {
			return Operations{}, fmt.Errorf("httpcontract: project operation errors contain stale route %q", route)
		}
	}
	for route := range operationIDs {
		if _, ok := routes[route]; !ok {
			return Operations{}, fmt.Errorf("httpcontract: project operation ids contain stale route %q", route)
		}
	}
	for route := range operationOverrides {
		if _, ok := routes[route]; !ok {
			return Operations{}, fmt.Errorf("httpcontract: project operation overrides contain stale route %q", route)
		}
	}
	sort.Slice(manifest.Operations, func(i, j int) bool { return manifest.Operations[i].ID < manifest.Operations[j].ID })
	if err := manifest.Validate(); err != nil {
		return Operations{}, err
	}
	return manifest, nil
}

func VerifyProjectCatalogCoverage(catalog ErrorCatalog, operations Operations, allowUnused ...string) error {
	if err := VerifyReferences(catalog, operations); err != nil {
		return err
	}
	used := make(map[string]struct{})
	allowed := make(map[string]struct{}, len(allowUnused))
	for _, code := range allowUnused {
		allowed[code] = struct{}{}
	}
	for _, operation := range operations.Operations {
		for _, code := range operation.Errors {
			used[code] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, definition := range catalog.Errors {
		if _, ok := used[definition.Code]; !ok {
			if _, exempt := allowed[definition.Code]; exempt {
				continue
			}
			missing = append(missing, definition.Code)
		}
	}
	for code := range allowed {
		found := false
		for _, definition := range catalog.Errors {
			if definition.Code == code {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("httpcontract: project allowUnusedErrors references undeclared code %q", code)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("httpcontract: project catalog errors are unused: %s", strings.Join(missing, ", "))
	}
	return nil
}

func GenerateLegacyCatalog(catalog ErrorCatalog, schemaVersion string) ([]byte, error) {
	type legacyError struct {
		Code   string `json:"code"`
		Status int    `json:"status"`
	}
	type legacyDocument struct {
		SchemaVersion string        `json:"schemaVersion"`
		Errors        []legacyError `json:"errors"`
	}
	result := legacyDocument{SchemaVersion: schemaVersion, Errors: make([]legacyError, 0, len(catalog.Errors))}
	for _, definition := range catalog.Errors {
		result.Errors = append(result.Errors, legacyError{Code: definition.Code, Status: definition.Status})
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func EncodeOperations(operations Operations) ([]byte, error) {
	data, err := json.MarshalIndent(operations, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
func projectSuccessResponse(responses map[string]openAPIResponse) (int, openAPIResponse, bool) {
	statuses := []int{}
	byStatus := map[int]openAPIResponse{}
	for raw, response := range responses {
		status, err := strconv.Atoi(raw)
		if err == nil && status >= 200 && status < 300 {
			statuses = append(statuses, status)
			byStatus[status] = response
		}
	}
	if len(statuses) == 0 {
		return 0, openAPIResponse{}, false
	}
	sort.Ints(statuses)
	return statuses[0], byStatus[statuses[0]], true
}
func projectSchemaRef(response openAPIResponse) string {
	types := make([]string, 0, len(response.Content))
	for value := range response.Content {
		types = append(types, value)
	}
	sort.Strings(types)
	for _, value := range types {
		if ref := response.Content[value].Schema.Ref; ref != "" {
			return ref
		}
	}
	return ""
}
func projectResponseKind(status int, ref string, schemas map[string]openAPISchema) string {
	if status == 204 {
		return "empty"
	}
	if status == 202 {
		return "operation"
	}
	properties := schemas[strings.TrimPrefix(ref, "#/components/schemas/")].Properties
	_, items := properties["items"]
	_, list := properties["list"]
	_, entries := properties["entries"]
	_, total := properties["total"]
	if total && (items || list || entries) {
		return "page"
	}
	if items || list || entries {
		return "collection"
	}
	return "resource"
}
func projectOperationID(namespace, method, path string) string {
	parts := []string{namespace, strings.ToLower(method)}
	for _, part := range strings.Split(strings.Trim(path, "/"), "/") {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			part = "by-" + strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
		}
		part = strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
				return r
			}
			return '-'
		}, part)
		parts = append(parts, part)
	}
	return strings.Join(parts, ".")
}

func EqualGenerated(current, generated []byte) bool {
	normalize := func(value []byte) []byte { return bytes.ReplaceAll(value, []byte("\r\n"), []byte("\n")) }
	return bytes.Equal(normalize(current), normalize(generated))
}
