package audit

import (
	"fmt"
	"slices"
	"time"
)

type EvidenceKind string

const (
	EvidenceCode          EvidenceKind = "code"
	EvidenceReference     EvidenceKind = "reference"
	EvidenceBool          EvidenceKind = "bool"
	EvidenceInt64         EvidenceKind = "int64"
	EvidenceCount         EvidenceKind = "count"
	EvidenceDigest        EvidenceKind = "digest"
	EvidenceTime          EvidenceKind = "time"
	EvidenceCodeList      EvidenceKind = "code_list"
	EvidenceReferenceList EvidenceKind = "reference_list"
)

type EvidenceField struct {
	Key   EvidenceKey  `json:"key"`
	Kind  EvidenceKind `json:"kind"`
	Text  string       `json:"text,omitempty"`
	Bool  *bool        `json:"bool,omitempty"`
	Int64 *int64       `json:"int64,omitempty"`
	Uint  *uint64      `json:"uint,omitempty"`
	Time  *time.Time   `json:"time,omitempty"`
	List  []string     `json:"list,omitempty"`
}

func Code(key EvidenceKey, value string) EvidenceField {
	return EvidenceField{Key: key, Kind: EvidenceCode, Text: value}
}

func Reference(key EvidenceKey, value string) EvidenceField {
	return EvidenceField{Key: key, Kind: EvidenceReference, Text: value}
}

func Bool(key EvidenceKey, value bool) EvidenceField {
	return EvidenceField{Key: key, Kind: EvidenceBool, Bool: &value}
}

func Int64(key EvidenceKey, value int64) EvidenceField {
	return EvidenceField{Key: key, Kind: EvidenceInt64, Int64: &value}
}

func Count(key EvidenceKey, value uint64) EvidenceField {
	return EvidenceField{Key: key, Kind: EvidenceCount, Uint: &value}
}

func EvidenceDigestValue(key EvidenceKey, value string) EvidenceField {
	return EvidenceField{Key: key, Kind: EvidenceDigest, Text: value}
}

func EvidenceTimeValue(key EvidenceKey, value time.Time) EvidenceField {
	value = value.UTC()
	return EvidenceField{Key: key, Kind: EvidenceTime, Time: &value}
}

func Codes(key EvidenceKey, values ...string) EvidenceField {
	return EvidenceField{Key: key, Kind: EvidenceCodeList, List: slices.Clone(values)}
}

func References(key EvidenceKey, values ...string) EvidenceField {
	return EvidenceField{Key: key, Kind: EvidenceReferenceList, List: slices.Clone(values)}
}

func (f EvidenceField) clone() EvidenceField {
	out := f
	out.List = slices.Clone(f.List)
	if f.Bool != nil {
		value := *f.Bool
		out.Bool = &value
	}
	if f.Int64 != nil {
		value := *f.Int64
		out.Int64 = &value
	}
	if f.Uint != nil {
		value := *f.Uint
		out.Uint = &value
	}
	if f.Time != nil {
		value := *f.Time
		out.Time = &value
	}
	return out
}

func validateEvidenceShape(field EvidenceField) error {
	switch field.Kind {
	case EvidenceCode, EvidenceReference, EvidenceDigest:
		if field.Text == "" || field.Bool != nil || field.Int64 != nil || field.Uint != nil || field.Time != nil || len(field.List) != 0 {
			return fmt.Errorf("requires exactly one non-empty text value")
		}
	case EvidenceBool:
		if field.Bool == nil || field.Text != "" || field.Int64 != nil || field.Uint != nil || field.Time != nil || len(field.List) != 0 {
			return fmt.Errorf("requires exactly one bool value")
		}
	case EvidenceInt64:
		if field.Int64 == nil || field.Text != "" || field.Bool != nil || field.Uint != nil || field.Time != nil || len(field.List) != 0 {
			return fmt.Errorf("requires exactly one int64 value")
		}
	case EvidenceCount:
		if field.Uint == nil || field.Text != "" || field.Bool != nil || field.Int64 != nil || field.Time != nil || len(field.List) != 0 {
			return fmt.Errorf("requires exactly one count value")
		}
	case EvidenceTime:
		if field.Time == nil || field.Text != "" || field.Bool != nil || field.Int64 != nil || field.Uint != nil || len(field.List) != 0 {
			return fmt.Errorf("requires exactly one time value")
		}
	case EvidenceCodeList, EvidenceReferenceList:
		if len(field.List) == 0 || field.Text != "" || field.Bool != nil || field.Int64 != nil || field.Uint != nil || field.Time != nil {
			return fmt.Errorf("requires a non-empty code list")
		}
	default:
		return fmt.Errorf("has unknown evidence kind")
	}
	return nil
}
