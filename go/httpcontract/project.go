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

type Project struct {
	SchemaVersion string                `json:"schemaVersion"`
	Namespace     string                `json:"namespace"`
	OpenAPI       ProjectOpenAPI        `json:"openapi"`
	ErrorCatalog  string                `json:"errorCatalog"`
	Operations    ProjectOperations     `json:"operations"`
	Generate      ProjectGenerate       `json:"generate"`
	LegacyCatalog *ProjectLegacyCatalog `json:"legacyCatalog,omitempty"`
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
	Output string              `json:"output"`
	Errors map[string][]string `json:"errors,omitempty"`
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
	return nil
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
	var document openAPIDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return Operations{}, fmt.Errorf("httpcontract: decode OpenAPI: %w", err)
	}
	manifest := Operations{SchemaVersion: OperationsSchemaVersion, Namespace: namespace}
	routes := make(map[string]struct{})
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
			manifest.Operations = append(manifest.Operations, Operation{ID: projectOperationID(namespace, method, path), Method: strings.ToUpper(method), Path: path, Success: Success{Status: status, Kind: projectResponseKind(status, ref, document.Components.Schemas), SchemaRef: ref}, Errors: operationErrors[key]})
		}
	}
	for route := range operationErrors {
		if _, ok := routes[route]; !ok {
			return Operations{}, fmt.Errorf("httpcontract: project operation errors contain stale route %q", route)
		}
	}
	sort.Slice(manifest.Operations, func(i, j int) bool { return manifest.Operations[i].ID < manifest.Operations[j].ID })
	if err := manifest.Validate(); err != nil {
		return Operations{}, err
	}
	return manifest, nil
}

func VerifyProjectCatalogCoverage(catalog ErrorCatalog, operations Operations) error {
	if err := VerifyReferences(catalog, operations); err != nil {
		return err
	}
	used := make(map[string]struct{})
	for _, operation := range operations.Operations {
		for _, code := range operation.Errors {
			used[code] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, definition := range catalog.Errors {
		if _, ok := used[definition.Code]; !ok {
			missing = append(missing, definition.Code)
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
