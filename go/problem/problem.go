// Package problem implements the framework-independent Yueli HTTP Problem v1
// contract. HTTP frameworks belong in adapter packages and must additionally
// enforce status, content-type, trace-header and body-size invariants.
package problem

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxTypeLength       = 2048
	maxCodeLength       = 128
	maxTraceIDLength    = 128
	maxParameterCount   = 32
	maxParameterKey     = 64
	maxParameterString  = 1024
	maxParameterArray   = 32
	maxViolationCount   = 64
	maxViolationPointer = 1024
)

var (
	codePattern      = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)+$`)
	traceIDPattern   = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	parameterPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
	pointerPattern   = regexp.MustCompile(`^(?:/(?:[^~/]|~[01])*)*$`)
)

// Parameters carries JSON-safe, caller-translated interpolation values.
// Nested objects, nulls and non-finite numbers are rejected by Validate.
type Parameters map[string]any

// Violation identifies one invalid input location using an RFC 6901 JSON
// Pointer and a stable caller-resolved code.
type Violation struct {
	Pointer string     `json:"pointer"`
	Code    string     `json:"code"`
	Params  Parameters `json:"params,omitempty"`
}

// Problem is the canonical failure body. Unknown JSON extension members are
// intentionally ignored when decoding, as permitted by the shared schema.
type Problem struct {
	Type       string      `json:"type"`
	Status     int         `json:"status"`
	Code       string      `json:"code"`
	Params     Parameters  `json:"params,omitempty"`
	Violations []Violation `json:"violations,omitempty"`
	TraceID    string      `json:"traceId"`
}

// Kind is an immutable code/status pair. It replaces mutable process-global
// registries; applications own their Kind values explicitly.
type Kind struct {
	code   string
	status int
}

// NewKind validates and returns an immutable Problem kind.
func NewKind(code string, status int) (Kind, error) {
	if err := validateCode(code); err != nil {
		return Kind{}, err
	}
	if err := validateStatus(status); err != nil {
		return Kind{}, err
	}
	return Kind{code: code, status: status}, nil
}

// MustKind is intended for package-level constants whose invalidity is a
// programmer error. It does not register global mutable state.
func MustKind(code string, status int) Kind {
	kind, err := NewKind(code, status)
	if err != nil {
		panic(err)
	}
	return kind
}

func (kind Kind) Code() string { return kind.code }

func (kind Kind) Status() int { return kind.status }

// New constructs and validates a Problem from an explicit immutable Kind.
func New(kind Kind, problemType, traceID string, params Parameters, violations ...Violation) (Problem, error) {
	value := Problem{
		Type:       problemType,
		Status:     kind.status,
		Code:       kind.code,
		Params:     params,
		Violations: violations,
		TraceID:    traceID,
	}
	if err := value.Validate(); err != nil {
		return Problem{}, err
	}
	return value, nil
}

// Decode parses one JSON object and validates the public contract. It rejects
// trailing JSON values while preserving JSON numbers for finite validation.
func Decode(data []byte) (Problem, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value Problem
	if err := decoder.Decode(&value); err != nil {
		return Problem{}, fmt.Errorf("problem: decode: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Problem{}, err
	}
	if err := value.Validate(); err != nil {
		return Problem{}, err
	}
	return value, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("problem: trailing JSON: %w", err)
	}
	return errors.New("problem: multiple JSON values")
}

// Validate applies the shared schema constraints that are independent of an
// HTTP response. Adapter-only invariants are documented by the contract.
func (value Problem) Validate() error {
	if utf8.RuneCountInString(value.Type) <= len("https://") || utf8.RuneCountInString(value.Type) > maxTypeLength || !strings.HasPrefix(value.Type, "https://") || strings.ContainsAny(value.Type, " \t\r\n") {
		return errors.New("problem: type must be a bounded HTTPS URI")
	}
	if err := validateStatus(value.Status); err != nil {
		return err
	}
	if err := validateCode(value.Code); err != nil {
		return err
	}
	if len(value.TraceID) == 0 || len(value.TraceID) > maxTraceIDLength || !traceIDPattern.MatchString(value.TraceID) {
		return errors.New("problem: traceId is invalid")
	}
	if err := validateParameters("params", value.Params); err != nil {
		return err
	}
	if len(value.Violations) > maxViolationCount {
		return errors.New("problem: too many violations")
	}
	for index, violation := range value.Violations {
		path := "violations[" + strconv.Itoa(index) + "]"
		if utf8.RuneCountInString(violation.Pointer) > maxViolationPointer || !pointerPattern.MatchString(violation.Pointer) {
			return fmt.Errorf("problem: %s.pointer is invalid", path)
		}
		if err := validateCode(violation.Code); err != nil {
			return fmt.Errorf("problem: %s.code: %w", path, err)
		}
		if err := validateParameters(path+".params", violation.Params); err != nil {
			return err
		}
	}
	return nil
}

func validateStatus(status int) error {
	if status < 400 || status > 599 {
		return errors.New("problem: status must be between 400 and 599")
	}
	return nil
}

func validateCode(code string) error {
	if len(code) < 3 || len(code) > maxCodeLength || !codePattern.MatchString(code) {
		return errors.New("problem: code is invalid")
	}
	return nil
}

func validateParameters(path string, params Parameters) error {
	if len(params) > maxParameterCount {
		return fmt.Errorf("problem: %s has too many properties", path)
	}
	for key, value := range params {
		if len(key) > maxParameterKey || !parameterPattern.MatchString(key) {
			return fmt.Errorf("problem: %s key %q is invalid", path, key)
		}
		if err := validateParameterValue(value); err != nil {
			return fmt.Errorf("problem: %s.%s: %w", path, key, err)
		}
	}
	return nil
}

func validateParameterValue(value any) error {
	if isScalar(value) {
		return nil
	}
	if value == nil {
		return errors.New("null is not allowed")
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice {
		return errors.New("nested values are not allowed")
	}
	if rv.Len() > maxParameterArray {
		return errors.New("array is too long")
	}
	for index := 0; index < rv.Len(); index++ {
		if !isScalar(rv.Index(index).Interface()) {
			return fmt.Errorf("array item %d is not a scalar", index)
		}
	}
	return nil
}

func isScalar(value any) bool {
	switch typed := value.(type) {
	case string:
		return utf8.RuneCountInString(typed) <= maxParameterString
	case bool:
		return true
	case json.Number:
		parsed, err := typed.Float64()
		return err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case float32:
		return !math.IsNaN(float64(typed)) && !math.IsInf(float64(typed), 0)
	case float64:
		return !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}
