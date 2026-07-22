package problem

import (
	"errors"
	"fmt"
)

const validationTraceID = "foundation-validation"

// Descriptor is an immutable public error contract. Applications declare
// descriptors instead of mutating a process-global code-to-status registry.
type Descriptor struct {
	kind        Kind
	problemType string
}

func NewDescriptor(kind Kind, problemType string) (Descriptor, error) {
	if _, err := New(kind, problemType, validationTraceID, nil); err != nil {
		return Descriptor{}, fmt.Errorf("problem: descriptor: %w", err)
	}
	return Descriptor{kind: kind, problemType: problemType}, nil
}

func MustDescriptor(kind Kind, problemType string) Descriptor {
	descriptor, err := NewDescriptor(kind, problemType)
	if err != nil {
		panic(err)
	}
	return descriptor
}

func (descriptor Descriptor) Kind() Kind { return descriptor.kind }

func (descriptor Descriptor) Type() string { return descriptor.problemType }

// Error copies its public data at construction, so later caller mutations do
// not change the wire mapping. A wrapped cause remains server diagnostic only.
type Error struct {
	descriptor Descriptor
	params     Parameters
	violations []Violation
	cause      error
}

func NewError(descriptor Descriptor, params Parameters, violations ...Violation) (*Error, error) {
	return newError(descriptor, nil, params, violations)
}

func WrapError(descriptor Descriptor, cause error, params Parameters, violations ...Violation) (*Error, error) {
	if cause == nil {
		return nil, errors.New("problem: wrapped cause is nil")
	}
	return newError(descriptor, cause, params, violations)
}

func newError(descriptor Descriptor, cause error, params Parameters, violations []Violation) (*Error, error) {
	paramsCopy := cloneParameters(params)
	violationsCopy := cloneViolations(violations)
	if _, err := New(descriptor.kind, descriptor.problemType, validationTraceID, paramsCopy, violationsCopy...); err != nil {
		return nil, fmt.Errorf("problem: error mapping: %w", err)
	}
	return &Error{descriptor: descriptor, params: paramsCopy, violations: violationsCopy, cause: cause}, nil
}

func (value *Error) Error() string {
	if value == nil {
		return "problem: nil error"
	}
	if value.cause != nil {
		return value.descriptor.kind.code + ": " + value.cause.Error()
	}
	return value.descriptor.kind.code
}

func (value *Error) Unwrap() error {
	if value == nil {
		return nil
	}
	return value.cause
}

// FromError resolves the first wrapped public Error and builds a fresh Problem
// for this request's trace ID. Unknown errors deliberately return ok=false.
func FromError(err error, traceID string) (result Problem, ok bool, resolveErr error) {
	var mapped *Error
	if !errors.As(err, &mapped) || mapped == nil {
		return Problem{}, false, nil
	}
	value, buildErr := New(mapped.descriptor.kind, mapped.descriptor.problemType, traceID, cloneParameters(mapped.params), cloneViolations(mapped.violations)...)
	if buildErr != nil {
		return Problem{}, true, buildErr
	}
	return value, true, nil
}

func cloneParameters(source Parameters) Parameters {
	if len(source) == 0 {
		return nil
	}
	result := make(Parameters, len(source))
	for key, value := range source {
		result[key] = cloneParameterValue(value)
	}
	return result
}

func cloneParameterValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []bool:
		return append([]bool(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []int64:
		return append([]int64(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case []any:
		return append([]any(nil), typed...)
	default:
		return value
	}
}

func cloneViolations(source []Violation) []Violation {
	if len(source) == 0 {
		return nil
	}
	result := make([]Violation, len(source))
	for index, violation := range source {
		result[index] = Violation{Pointer: violation.Pointer, Code: violation.Code, Params: cloneParameters(violation.Params)}
	}
	return result
}
