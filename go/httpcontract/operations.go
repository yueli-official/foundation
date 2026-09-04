package httpcontract

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const OperationsSchemaVersion = "http.yueli.dev/operations/v1"

var (
	operationIDPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9._-]{1,127}$`)
	oauthErrorPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
)

type Operations struct {
	SchemaVersion string      `json:"schemaVersion"`
	Namespace     string      `json:"namespace"`
	Operations    []Operation `json:"operations"`
}

type Operation struct {
	ID              string   `json:"id"`
	Method          string   `json:"method"`
	Path            string   `json:"path"`
	FailureProtocol string   `json:"failureProtocol,omitempty"`
	Success         Success  `json:"success"`
	Errors          []string `json:"errors,omitempty"`
}

type Success struct {
	Status    int    `json:"status"`
	Kind      string `json:"kind"`
	SchemaRef string `json:"schemaRef,omitempty"`
}

func ParseOperations(data []byte) (Operations, error) {
	var operations Operations
	if err := decodeStrict(data, &operations); err != nil {
		return Operations{}, fmt.Errorf("httpcontract: decode operations: %w", err)
	}
	if err := operations.Validate(); err != nil {
		return Operations{}, err
	}
	return operations, nil
}

func (manifest Operations) Validate() error {
	if manifest.SchemaVersion != OperationsSchemaVersion {
		return fmt.Errorf("httpcontract: unsupported operations schemaVersion %q", manifest.SchemaVersion)
	}
	if !namespacePattern.MatchString(manifest.Namespace) {
		return errors.New("httpcontract: operations namespace is invalid")
	}
	if len(manifest.Operations) == 0 {
		return errors.New("httpcontract: operations manifest requires at least one operation")
	}
	seen := make(map[string]struct{}, len(manifest.Operations))
	for index, operation := range manifest.Operations {
		location := fmt.Sprintf("operations[%d]", index)
		if !operationIDPattern.MatchString(operation.ID) || !strings.HasPrefix(operation.ID, manifest.Namespace+".") {
			return fmt.Errorf("httpcontract: %s.id must belong to namespace %q", location, manifest.Namespace)
		}
		if _, exists := seen[operation.ID]; exists {
			return fmt.Errorf("httpcontract: duplicate operation id %q", operation.ID)
		}
		seen[operation.ID] = struct{}{}
		if !validMethod(operation.Method) {
			return fmt.Errorf("httpcontract: %s.method is invalid", location)
		}
		if !strings.HasPrefix(operation.Path, "/") || len(operation.Path) > 2048 {
			return fmt.Errorf("httpcontract: %s.path is invalid", location)
		}
		if err := operation.Success.validate(); err != nil {
			return fmt.Errorf("httpcontract: %s.success: %w", location, err)
		}
		protocol := operation.FailureProtocol
		if protocol == "" {
			protocol = "problem"
		}
		if protocol != "problem" && protocol != "oauth" {
			return fmt.Errorf("httpcontract: %s.failureProtocol is invalid", location)
		}
		errorSeen := map[string]struct{}{}
		for _, code := range operation.Errors {
			valid := codePattern.MatchString(code)
			if protocol == "oauth" {
				valid = oauthErrorPattern.MatchString(code)
			}
			if !valid {
				return fmt.Errorf("httpcontract: %s.errors contains invalid code %q", location, code)
			}
			if _, exists := errorSeen[code]; exists {
				return fmt.Errorf("httpcontract: %s.errors contains duplicate code %q", location, code)
			}
			errorSeen[code] = struct{}{}
		}
	}
	return nil
}

func (success Success) validate() error {
	needsSchema := false
	switch success.Kind {
	case "resource":
		needsSchema = true
		if success.Status != 200 && success.Status != 201 {
			return errors.New("resource status must be 200 or 201")
		}
	case "collection", "page", "cursorPage":
		needsSchema = true
		if success.Status != 200 {
			return errors.New("collection status must be 200")
		}
	case "operation":
		needsSchema = true
		if success.Status != 202 {
			return errors.New("operation status must be 202")
		}
	case "empty":
		if success.Status != 204 {
			return errors.New("empty status must be 204")
		}
	case "binary":
		if success.Status < 200 || success.Status > 299 || success.Status == 204 {
			return errors.New("binary status must be a body-bearing 2xx")
		}
	case "redirect":
		if success.Status < 300 || success.Status > 399 {
			return errors.New("redirect status must be 3xx")
		}
	default:
		return errors.New("kind is invalid")
	}
	if needsSchema && success.SchemaRef == "" {
		return errors.New("schemaRef is required")
	}
	if !needsSchema && success.SchemaRef != "" {
		return errors.New("schemaRef is not allowed")
	}
	return nil
}

func validMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}
