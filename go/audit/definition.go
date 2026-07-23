package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

type CommitRequirement string

const (
	CommitAtomicRequired   CommitRequirement = "atomic_required"
	CommitIndependentAllow CommitRequirement = "independent_allowed"
)

type Category string

const (
	CategoryAuthorization  Category = "authorization"
	CategoryAdministration Category = "administration"
	CategoryConfiguration  Category = "configuration"
	CategorySecurity       Category = "security"
	CategoryExport         Category = "export"
	CategoryAuditLifecycle Category = "audit_lifecycle"
)

type FieldDefinition struct {
	Key       EvidenceKey
	Kind      EvidenceKind
	Required  bool
	MaxItems  int
	MaxLength int
}

type ActionDefinition struct {
	Action      Action
	Category    Category
	TargetTypes []string
	Commit      CommitRequirement
	Retention   RetentionClass
	Evidence    []FieldDefinition
}

type RetentionDefinition struct {
	Class         RetentionClass
	MinimumAge    time.Duration
	ArchiveBefore bool
}

type Definition struct {
	Version     uint64
	Consumer    string
	Actions     []ActionDefinition
	Retention   []RetentionDefinition
	MaxBatch    int
	MaxEvidence int
}

type compiledAction struct {
	definition ActionDefinition
	fields     map[EvidenceKey]FieldDefinition
	digest     Digest
}

type Catalog struct {
	definition Definition
	actions    map[Action]compiledAction
	retention  map[RetentionClass]RetentionDefinition
	digest     Digest
}

var (
	namePattern      = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)+$`)
	codePattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]*$`)
	hexDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	traceIDPattern   = regexp.MustCompile(`^[a-f0-9]{32}$`)
	spanIDPattern    = regexp.MustCompile(`^[a-f0-9]{16}$`)
	bannedEvidence   = []string{
		"password", "passwd", "secret", "token", "cookie", "session",
		"credential", "connection_string", "private_key", "request_body", "response_body",
	}
)

func Compile(in Definition) (*Catalog, error) {
	if in.Version == 0 {
		return nil, invalidDefinition("version", "must be positive")
	}
	in.Consumer = strings.TrimSpace(in.Consumer)
	if !namePattern.MatchString(in.Consumer) {
		return nil, invalidDefinition("consumer", "must be a namespaced stable name")
	}
	if in.MaxBatch == 0 {
		in.MaxBatch = 100
	}
	if in.MaxEvidence == 0 {
		in.MaxEvidence = 32
	}
	if in.MaxBatch < 1 || in.MaxBatch > 1000 {
		return nil, invalidDefinition("max_batch", "must be between 1 and 1000")
	}
	if in.MaxEvidence < 1 || in.MaxEvidence > 128 {
		return nil, invalidDefinition("max_evidence", "must be between 1 and 128")
	}
	catalog := &Catalog{
		definition: in,
		actions:    make(map[Action]compiledAction, len(in.Actions)),
		retention:  make(map[RetentionClass]RetentionDefinition, len(in.Retention)),
	}
	for i := range in.Retention {
		item := in.Retention[i]
		if !namePattern.MatchString(string(item.Class)) {
			return nil, invalidDefinition(fmt.Sprintf("retention[%d].class", i), "must be a namespaced stable name")
		}
		if item.MinimumAge <= 0 {
			return nil, invalidDefinition(fmt.Sprintf("retention[%d].minimum_age", i), "must be positive")
		}
		if _, exists := catalog.retention[item.Class]; exists {
			return nil, invalidDefinition(fmt.Sprintf("retention[%d].class", i), "is duplicated")
		}
		catalog.retention[item.Class] = item
	}
	for i := range in.Actions {
		item := in.Actions[i]
		if !namePattern.MatchString(string(item.Action.Name)) || item.Action.Version == 0 {
			return nil, invalidDefinition(fmt.Sprintf("actions[%d].action", i), "must have a namespaced name and positive version")
		}
		if item.Commit == "" {
			item.Commit = CommitAtomicRequired
		}
		if item.Commit != CommitAtomicRequired && item.Commit != CommitIndependentAllow {
			return nil, invalidDefinition(fmt.Sprintf("actions[%d].commit", i), "is invalid")
		}
		if _, exists := catalog.retention[item.Retention]; !exists {
			return nil, invalidDefinition(fmt.Sprintf("actions[%d].retention", i), "is not declared")
		}
		if len(item.TargetTypes) == 0 {
			return nil, invalidDefinition(fmt.Sprintf("actions[%d].target_types", i), "must not be empty")
		}
		for _, targetType := range item.TargetTypes {
			if !namePattern.MatchString(targetType) {
				return nil, invalidDefinition(fmt.Sprintf("actions[%d].target_types", i), "contains an invalid target type")
			}
		}
		fields := make(map[EvidenceKey]FieldDefinition, len(item.Evidence))
		for fieldIndex := range item.Evidence {
			field := item.Evidence[fieldIndex]
			key := strings.ToLower(string(field.Key))
			if !namePattern.MatchString(key) {
				return nil, invalidDefinition(fmt.Sprintf("actions[%d].evidence[%d].key", i, fieldIndex), "must be a namespaced stable name")
			}
			for _, banned := range bannedEvidence {
				if strings.Contains(key, banned) {
					return nil, invalidDefinition(fmt.Sprintf("actions[%d].evidence[%d].key", i, fieldIndex), "is reserved for sensitive data")
				}
			}
			if _, exists := fields[field.Key]; exists {
				return nil, invalidDefinition(fmt.Sprintf("actions[%d].evidence[%d].key", i, fieldIndex), "is duplicated")
			}
			if field.MaxLength == 0 {
				field.MaxLength = 256
			}
			if field.MaxItems == 0 {
				field.MaxItems = 32
			}
			if field.MaxLength < 1 || field.MaxLength > 4096 || field.MaxItems < 1 || field.MaxItems > 128 {
				return nil, invalidDefinition(fmt.Sprintf("actions[%d].evidence[%d]", i, fieldIndex), "limits are invalid")
			}
			fields[field.Key] = field
			item.Evidence[fieldIndex] = field
		}
		if _, exists := catalog.actions[item.Action]; exists {
			return nil, invalidDefinition(fmt.Sprintf("actions[%d].action", i), "is duplicated")
		}
		item.TargetTypes = slices.Clone(item.TargetTypes)
		item.Evidence = slices.Clone(item.Evidence)
		actionJSON, err := json.Marshal(item)
		if err != nil {
			return nil, invalidDefinition(fmt.Sprintf("actions[%d]", i), "cannot be encoded")
		}
		actionSum := sha256.Sum256(actionJSON)
		catalog.actions[item.Action] = compiledAction{
			definition: item,
			fields:     fields,
			digest:     Digest(hex.EncodeToString(actionSum[:])),
		}
		in.Actions[i] = item
	}
	if len(catalog.actions) == 0 {
		return nil, invalidDefinition("actions", "must not be empty")
	}
	catalog.definition = in
	encoded, err := json.Marshal(in)
	if err != nil {
		return nil, invalidDefinition("definition", "cannot be encoded")
	}
	sum := sha256.Sum256(encoded)
	catalog.digest = Digest(hex.EncodeToString(sum[:]))
	return catalog, nil
}

func MustCompile(in Definition) *Catalog {
	catalog, err := Compile(in)
	if err != nil {
		panic(err)
	}
	return catalog
}

func (c *Catalog) Digest() Digest {
	if c == nil {
		return ""
	}
	return c.digest
}

func (c *Catalog) Definition() Definition {
	if c == nil {
		return Definition{}
	}
	out := c.definition
	out.Actions = slices.Clone(c.definition.Actions)
	out.Retention = slices.Clone(c.definition.Retention)
	for i := range out.Actions {
		out.Actions[i].TargetTypes = slices.Clone(out.Actions[i].TargetTypes)
		out.Actions[i].Evidence = slices.Clone(out.Actions[i].Evidence)
	}
	return out
}
