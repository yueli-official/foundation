// Package httpcontract validates product-owned public error catalogs and HTTP
// operation result declarations. Product DTO schemas remain in OpenAPI; this
// package owns only stable error semantics and response shape invariants.
package httpcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const ErrorCatalogSchemaVersion = "errors.yueli.dev/catalog/v1"

var (
	namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
	codePattern      = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)+$`)
	keyPattern       = regexp.MustCompile(`^[a-z][A-Za-z0-9._-]+$`)
	paramPattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
)

type ErrorCatalog struct {
	SchemaVersion string            `json:"schemaVersion"`
	Namespace     string            `json:"namespace"`
	Errors        []ErrorDefinition `json:"errors"`
}

type ErrorDefinition struct {
	Code        string                         `json:"code"`
	GoName      string                         `json:"goName,omitempty"`
	Status      int                            `json:"status"`
	MessageKey  string                         `json:"messageKey"`
	RecoveryKey string                         `json:"recoveryKey,omitempty"`
	Params      map[string]ParameterDefinition `json:"params,omitempty"`
	Violations  string                         `json:"violations,omitempty"`
}

type ParameterDefinition struct {
	Type      string `json:"type"`
	Required  bool   `json:"required,omitempty"`
	MaxLength int    `json:"maxLength,omitempty"`
	MaxItems  int    `json:"maxItems,omitempty"`
}

func ParseErrorCatalog(data []byte) (ErrorCatalog, error) {
	var catalog ErrorCatalog
	if err := decodeStrict(data, &catalog); err != nil {
		return ErrorCatalog{}, fmt.Errorf("httpcontract: decode error catalog: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return ErrorCatalog{}, err
	}
	return catalog, nil
}

func (catalog ErrorCatalog) Validate() error {
	if catalog.SchemaVersion != ErrorCatalogSchemaVersion {
		return fmt.Errorf("httpcontract: unsupported error catalog schemaVersion %q", catalog.SchemaVersion)
	}
	if !namespacePattern.MatchString(catalog.Namespace) {
		return errors.New("httpcontract: error catalog namespace is invalid")
	}
	if len(catalog.Errors) == 0 {
		return errors.New("httpcontract: error catalog requires at least one error")
	}
	seen := make(map[string]struct{}, len(catalog.Errors))
	for index, definition := range catalog.Errors {
		location := fmt.Sprintf("errors[%d]", index)
		if !codePattern.MatchString(definition.Code) || !strings.HasPrefix(definition.Code, catalog.Namespace+".") {
			return fmt.Errorf("httpcontract: %s.code must belong to namespace %q", location, catalog.Namespace)
		}
		if _, exists := seen[definition.Code]; exists {
			return fmt.Errorf("httpcontract: duplicate error code %q", definition.Code)
		}
		seen[definition.Code] = struct{}{}
		if definition.Status < 400 || definition.Status > 599 {
			return fmt.Errorf("httpcontract: %s.status must be between 400 and 599", location)
		}
		if !keyPattern.MatchString(definition.MessageKey) {
			return fmt.Errorf("httpcontract: %s.messageKey is invalid", location)
		}
		if definition.GoName != "" && !regexpTypeName.MatchString(definition.GoName) {
			return fmt.Errorf("httpcontract: %s.goName is invalid", location)
		}
		if definition.RecoveryKey != "" && !keyPattern.MatchString(definition.RecoveryKey) {
			return fmt.Errorf("httpcontract: %s.recoveryKey is invalid", location)
		}
		if definition.Violations == "" {
			definition.Violations = "forbidden"
		}
		if definition.Violations != "forbidden" && definition.Violations != "optional" && definition.Violations != "required" {
			return fmt.Errorf("httpcontract: %s.violations is invalid", location)
		}
		for name, parameter := range definition.Params {
			if !paramPattern.MatchString(name) {
				return fmt.Errorf("httpcontract: %s.params key %q is invalid", location, name)
			}
			if err := parameter.validate(); err != nil {
				return fmt.Errorf("httpcontract: %s.params.%s: %w", location, name, err)
			}
		}
	}
	return nil
}

func (definition ParameterDefinition) validate() error {
	array := strings.HasSuffix(definition.Type, "[]")
	base := strings.TrimSuffix(definition.Type, "[]")
	if base != "string" && base != "integer" && base != "number" && base != "boolean" {
		return errors.New("type is invalid")
	}
	if definition.MaxLength < 0 || definition.MaxLength > 1024 || (definition.MaxLength > 0 && base != "string") {
		return errors.New("maxLength is invalid for type")
	}
	if definition.MaxItems < 0 || definition.MaxItems > 32 || (definition.MaxItems > 0 && !array) {
		return errors.New("maxItems is invalid for type")
	}
	return nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
