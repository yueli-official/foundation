package privacy

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

const DefinitionVersion uint64 = 1
const OwnerProtocolVersion uint64 = 1

var definitionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,95}$`)

type SubjectKindDefinition struct {
	Key         SubjectKind `json:"key"`
	Description string      `json:"description,omitempty"`
	MaxRefBytes int         `json:"maxRefBytes,omitempty"`
}

type DataCategoryDefinition struct {
	Key         DataCategoryKey `json:"key"`
	Description string          `json:"description,omitempty"`
	Sensitive   bool            `json:"sensitive,omitempty"`
}

type NoticeDefinition struct {
	Ref           NoticeRef    `json:"ref"`
	ContentDigest string       `json:"contentDigest"`
	Locale        string       `json:"locale,omitempty"`
	Purposes      []PurposeRef `json:"purposes"`
	PublishedAt   time.Time    `json:"publishedAt"`
}

type SignalEffect string

const (
	SignalDeny     SignalEffect = "deny"
	SignalRestrict SignalEffect = "restrict"
)

type SignalDefinition struct {
	Key            SignalKey     `json:"key"`
	Description    string        `json:"description,omitempty"`
	MaxEvidenceAge time.Duration `json:"maxEvidenceAge,omitempty"`
}

type SignalRule struct {
	Signal       SignalKey        `json:"signal"`
	Effect       SignalEffect     `json:"effect"`
	Restrictions []RestrictionKey `json:"restrictions,omitempty"`
}

type RestrictionDefinition struct {
	Key         RestrictionKey `json:"key"`
	Description string         `json:"description,omitempty"`
}

type PurposeDefinition struct {
	Ref          PurposeRef              `json:"ref"`
	Basis        ProcessingBasis         `json:"basis"`
	Categories   []DataCategoryKey       `json:"categories"`
	Notices      []NoticeRef             `json:"notices,omitempty"`
	SignalRules  []SignalRule            `json:"signalRules,omitempty"`
	Restrictions []RestrictionDefinition `json:"restrictions,omitempty"`
	EffectiveAt  time.Time               `json:"effectiveAt,omitempty"`
	RetiredAt    *time.Time              `json:"retiredAt,omitempty"`
}

type ActivePurpose struct {
	Key PurposeKey `json:"key"`
	Ref PurposeRef `json:"ref"`
}

type ReviewOutcome string

const (
	ReviewOwnerDecision ReviewOutcome = "owner_decision"
	ReviewAnonymize     ReviewOutcome = "anonymize"
	ReviewDelete        ReviewOutcome = "delete"
)

type RetentionRuleDefinition struct {
	Ref                  RetentionRuleRef    `json:"ref"`
	Categories           []DataCategoryKey   `json:"categories"`
	Trigger              RetentionTriggerKey `json:"trigger"`
	ReviewAfter          CalendarPeriod      `json:"reviewAfter"`
	MaximumDelay         *CalendarPeriod     `json:"maximumDelay,omitempty"`
	DefaultReviewOutcome ReviewOutcome       `json:"defaultReviewOutcome"`
}

type DatasetDefinition struct {
	Key            DatasetKey         `json:"key"`
	Categories     []DataCategoryKey  `json:"categories"`
	Operations     []RightsOperation  `json:"operations"`
	RetentionRules []RetentionRuleRef `json:"retentionRules,omitempty"`
}

type OwnerDefinition struct {
	Ref          OwnerRef            `json:"ref"`
	SubjectKinds []SubjectKind       `json:"subjectKinds"`
	Datasets     []DatasetDefinition `json:"datasets"`
	// FinalizeAfterOwners delays this Owner until every non-finalizing Owner
	// has reached a terminal state. Account/identity Owners use this to avoid
	// revoking the subject's access while downstream work is still retryable.
	FinalizeAfterOwners bool `json:"finalizeAfterOwners,omitempty"`
}

type RightsPolicy struct {
	Operation          RightsOperation `json:"operation"`
	RespondWithin      CalendarPeriod  `json:"respondWithin"`
	VerificationMaxAge time.Duration   `json:"verificationMaxAge"`
}

type CoordinationDefinition struct {
	Owners         []OwnerDefinition `json:"owners"`
	RightsPolicies []RightsPolicy    `json:"rightsPolicies"`
}

type Limits struct {
	MaxReferenceBytes     int           `json:"maxReferenceBytes,omitempty"`
	MaxIdempotencyBytes   int           `json:"maxIdempotencyBytes,omitempty"`
	MaxAliases            int           `json:"maxAliases,omitempty"`
	MaxPurposesPerReceipt int           `json:"maxPurposesPerReceipt,omitempty"`
	MaxDuePage            int           `json:"maxDuePage,omitempty"`
	MaxDriveAttempts      int           `json:"maxDriveAttempts,omitempty"`
	MaxDriveDuration      time.Duration `json:"maxDriveDuration,omitempty"`
	DefaultRetryDelay     time.Duration `json:"defaultRetryDelay,omitempty"`
	TaskLease             time.Duration `json:"taskLease,omitempty"`
}

type Definition struct {
	Version        uint64                    `json:"version"`
	Consumer       string                    `json:"consumer"`
	SubjectKinds   []SubjectKindDefinition   `json:"subjectKinds"`
	DataCategories []DataCategoryDefinition  `json:"dataCategories"`
	Notices        []NoticeDefinition        `json:"notices,omitempty"`
	Purposes       []PurposeDefinition       `json:"purposes,omitempty"`
	ActivePurposes []ActivePurpose           `json:"activePurposes,omitempty"`
	Signals        []SignalDefinition        `json:"signals,omitempty"`
	RetentionRules []RetentionRuleDefinition `json:"retentionRules,omitempty"`
	Owner          *OwnerDefinition          `json:"owner,omitempty"`
	Coordination   *CoordinationDefinition   `json:"coordination,omitempty"`
	Limits         Limits                    `json:"limits,omitempty"`
}

type Catalog struct {
	version      uint64
	consumer     string
	digest       string
	subjectKinds map[SubjectKind]SubjectKindDefinition
	categories   map[DataCategoryKey]DataCategoryDefinition
	notices      map[NoticeRef]NoticeDefinition
	purposes     map[PurposeRef]PurposeDefinition
	active       map[PurposeKey]PurposeRef
	signals      map[SignalKey]SignalDefinition
	retention    map[RetentionRuleRef]RetentionRuleDefinition
	owner        *OwnerDefinition
	owners       map[OwnerKey]OwnerDefinition
	rights       map[RightsOperation]RightsPolicy
	limits       Limits
}

func Compile(definition Definition) (*Catalog, error) {
	if definition.Version != DefinitionVersion {
		return nil, invalid("version", fmt.Sprintf("must equal %d", DefinitionVersion))
	}
	definition.Consumer = strings.TrimSpace(definition.Consumer)
	if !validKey(definition.Consumer) {
		return nil, invalid("consumer", "must be a stable lowercase key")
	}
	limits, err := normalizeLimits(definition.Limits)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{
		version: definition.Version, consumer: definition.Consumer, limits: limits,
		subjectKinds: map[SubjectKind]SubjectKindDefinition{},
		categories:   map[DataCategoryKey]DataCategoryDefinition{},
		notices:      map[NoticeRef]NoticeDefinition{}, purposes: map[PurposeRef]PurposeDefinition{},
		active: map[PurposeKey]PurposeRef{}, signals: map[SignalKey]SignalDefinition{},
		retention: map[RetentionRuleRef]RetentionRuleDefinition{},
		owners:    map[OwnerKey]OwnerDefinition{}, rights: map[RightsOperation]RightsPolicy{},
	}
	if len(definition.SubjectKinds) == 0 {
		return nil, invalid("subject_kinds", "must contain at least one kind")
	}
	for index, item := range definition.SubjectKinds {
		if !validKey(string(item.Key)) {
			return nil, invalid("subject_kinds", fmt.Sprintf("item %d has an invalid key", index))
		}
		if item.MaxRefBytes == 0 {
			item.MaxRefBytes = limits.MaxReferenceBytes
		}
		if item.MaxRefBytes < 1 || item.MaxRefBytes > limits.MaxReferenceBytes {
			return nil, invalid("subject_kinds", fmt.Sprintf("item %d maxRefBytes is invalid", index))
		}
		if _, exists := catalog.subjectKinds[item.Key]; exists {
			return nil, invalid("subject_kinds", fmt.Sprintf("contains duplicate %q", item.Key))
		}
		catalog.subjectKinds[item.Key] = item
		definition.SubjectKinds[index] = item
	}
	for index, item := range definition.DataCategories {
		if !validKey(string(item.Key)) {
			return nil, invalid("data_categories", fmt.Sprintf("item %d has an invalid key", index))
		}
		if _, exists := catalog.categories[item.Key]; exists {
			return nil, invalid("data_categories", fmt.Sprintf("contains duplicate %q", item.Key))
		}
		catalog.categories[item.Key] = item
	}
	for index, item := range definition.Signals {
		if !validKey(string(item.Key)) || item.MaxEvidenceAge < 0 {
			return nil, invalid("signals", fmt.Sprintf("item %d is invalid", index))
		}
		if _, exists := catalog.signals[item.Key]; exists {
			return nil, invalid("signals", fmt.Sprintf("contains duplicate %q", item.Key))
		}
		catalog.signals[item.Key] = item
	}
	for index, item := range definition.Notices {
		if !validNoticeRef(item.Ref) || strings.TrimSpace(item.ContentDigest) == "" || item.PublishedAt.IsZero() {
			return nil, invalid("notices", fmt.Sprintf("item %d is invalid", index))
		}
		if _, exists := catalog.notices[item.Ref]; exists {
			return nil, invalid("notices", fmt.Sprintf("contains duplicate %v", item.Ref))
		}
		catalog.notices[item.Ref] = item
	}
	for index, item := range definition.Purposes {
		if !validPurposeRef(item.Ref) || !validBasis(item.Basis) || len(item.Categories) == 0 {
			return nil, invalid("purposes", fmt.Sprintf("item %d is invalid", index))
		}
		if _, exists := catalog.purposes[item.Ref]; exists {
			return nil, invalid("purposes", fmt.Sprintf("contains duplicate %v", item.Ref))
		}
		for _, category := range item.Categories {
			if _, exists := catalog.categories[category]; !exists {
				return nil, invalid("purposes", fmt.Sprintf("%v references unknown category %q", item.Ref, category))
			}
		}
		restrictions := map[RestrictionKey]struct{}{}
		for _, value := range item.Restrictions {
			if !validKey(string(value.Key)) {
				return nil, invalid("purposes", fmt.Sprintf("%v has invalid restriction", item.Ref))
			}
			restrictions[value.Key] = struct{}{}
		}
		for _, rule := range item.SignalRules {
			if _, exists := catalog.signals[rule.Signal]; !exists || (rule.Effect != SignalDeny && rule.Effect != SignalRestrict) {
				return nil, invalid("purposes", fmt.Sprintf("%v has invalid signal rule", item.Ref))
			}
			if rule.Effect == SignalRestrict && len(rule.Restrictions) == 0 {
				return nil, invalid("purposes", fmt.Sprintf("%v restriction signal has no restrictions", item.Ref))
			}
			for _, key := range rule.Restrictions {
				if _, exists := restrictions[key]; !exists {
					return nil, invalid("purposes", fmt.Sprintf("%v references unknown restriction %q", item.Ref, key))
				}
			}
		}
		if item.Basis == BasisConsent && len(item.Notices) == 0 {
			return nil, invalid("purposes", fmt.Sprintf("%v consent basis requires a notice", item.Ref))
		}
		catalog.purposes[item.Ref] = item
	}
	for ref, purpose := range catalog.purposes {
		for _, noticeRef := range purpose.Notices {
			notice, exists := catalog.notices[noticeRef]
			if !exists || !slices.Contains(notice.Purposes, ref) {
				return nil, invalid("purposes", fmt.Sprintf("%v notice %v does not include the exact purpose", ref, noticeRef))
			}
		}
	}
	for _, item := range definition.ActivePurposes {
		if item.Key != item.Ref.Key {
			return nil, invalid("active_purposes", "key must match purpose reference key")
		}
		if _, exists := catalog.purposes[item.Ref]; !exists {
			return nil, invalid("active_purposes", fmt.Sprintf("references unknown purpose %v", item.Ref))
		}
		if _, exists := catalog.active[item.Key]; exists {
			return nil, invalid("active_purposes", fmt.Sprintf("contains duplicate %q", item.Key))
		}
		catalog.active[item.Key] = item.Ref
	}
	for index, item := range definition.RetentionRules {
		if !validRetentionRef(item.Ref) || !validKey(string(item.Trigger)) || len(item.Categories) == 0 ||
			!validPeriod(item.ReviewAfter) || !validReviewOutcome(item.DefaultReviewOutcome) {
			return nil, invalid("retention_rules", fmt.Sprintf("item %d is invalid", index))
		}
		for _, category := range item.Categories {
			if _, exists := catalog.categories[category]; !exists {
				return nil, invalid("retention_rules", fmt.Sprintf("%v references unknown category %q", item.Ref, category))
			}
		}
		if item.MaximumDelay != nil && !validPeriod(*item.MaximumDelay) {
			return nil, invalid("retention_rules", fmt.Sprintf("%v maximum delay is invalid", item.Ref))
		}
		if _, exists := catalog.retention[item.Ref]; exists {
			return nil, invalid("retention_rules", fmt.Sprintf("contains duplicate %v", item.Ref))
		}
		catalog.retention[item.Ref] = item
	}
	if definition.Owner != nil {
		owner, err := catalog.validateOwner(*definition.Owner)
		if err != nil {
			return nil, fmt.Errorf("privacy: owner: %w", err)
		}
		catalog.owner = &owner
		definition.Owner = &owner
	}
	if definition.Coordination != nil {
		for index, item := range definition.Coordination.Owners {
			owner, err := catalog.validateOwner(item)
			if err != nil {
				return nil, fmt.Errorf("privacy: coordination owner %d: %w", index, err)
			}
			if _, exists := catalog.owners[owner.Ref.Key]; exists {
				return nil, invalid("coordination.owners", fmt.Sprintf("contains duplicate %q", owner.Ref.Key))
			}
			catalog.owners[owner.Ref.Key] = owner
			definition.Coordination.Owners[index] = owner
		}
		for index, policy := range definition.Coordination.RightsPolicies {
			if !validRightsOperation(policy.Operation) || !validPeriod(policy.RespondWithin) || policy.VerificationMaxAge <= 0 {
				return nil, invalid("coordination.rights_policies", fmt.Sprintf("item %d is invalid", index))
			}
			if _, exists := catalog.rights[policy.Operation]; exists {
				return nil, invalid("coordination.rights_policies", fmt.Sprintf("contains duplicate %q", policy.Operation))
			}
			catalog.rights[policy.Operation] = policy
		}
	}
	definition.Limits = limits
	canonicalizeDefinition(&definition)
	encoded, err := json.Marshal(definition)
	if err != nil {
		return nil, &Error{Kind: ErrorStoreUnavailable, Field: "definition", Message: "cannot encode canonical definition", Cause: err}
	}
	sum := sha256.Sum256(encoded)
	catalog.digest = hex.EncodeToString(sum[:])
	return catalog, nil
}

func MustCompile(definition Definition) *Catalog {
	catalog, err := Compile(definition)
	if err != nil {
		panic(err)
	}
	return catalog
}

func normalizeLimits(value Limits) (Limits, error) {
	if value.MaxReferenceBytes == 0 {
		value.MaxReferenceBytes = 512
	}
	if value.MaxIdempotencyBytes == 0 {
		value.MaxIdempotencyBytes = 200
	}
	if value.MaxAliases == 0 {
		value.MaxAliases = 8
	}
	if value.MaxPurposesPerReceipt == 0 {
		value.MaxPurposesPerReceipt = 32
	}
	if value.MaxDuePage == 0 {
		value.MaxDuePage = 200
	}
	if value.MaxDriveAttempts == 0 {
		value.MaxDriveAttempts = 8
	}
	if value.MaxDriveDuration == 0 {
		value.MaxDriveDuration = 30 * time.Second
	}
	if value.DefaultRetryDelay == 0 {
		value.DefaultRetryDelay = time.Minute
	}
	if value.TaskLease == 0 {
		value.TaskLease = time.Minute
	}
	if value.MaxReferenceBytes < 32 || value.MaxReferenceBytes > 4096 ||
		value.MaxIdempotencyBytes < 16 || value.MaxIdempotencyBytes > 1024 ||
		value.MaxAliases < 0 || value.MaxAliases > 64 ||
		value.MaxPurposesPerReceipt < 1 || value.MaxPurposesPerReceipt > 256 ||
		value.MaxDuePage < 1 || value.MaxDuePage > 1000 ||
		value.MaxDriveAttempts < 1 || value.MaxDriveAttempts > 100 ||
		value.MaxDriveDuration < time.Millisecond || value.MaxDriveDuration > 5*time.Minute ||
		value.DefaultRetryDelay < time.Second || value.TaskLease < time.Second {
		return Limits{}, invalid("limits", "contains an out-of-range value")
	}
	return value, nil
}

func (catalog *Catalog) validateOwner(value OwnerDefinition) (OwnerDefinition, error) {
	if !validKey(string(value.Ref.Key)) || value.Ref.Revision == 0 || len(value.Datasets) == 0 {
		return OwnerDefinition{}, invalid("definition", "is invalid")
	}
	for _, kind := range value.SubjectKinds {
		if _, exists := catalog.subjectKinds[kind]; !exists {
			return OwnerDefinition{}, invalid("subject_kinds", fmt.Sprintf("references unknown %q", kind))
		}
	}
	seen := map[DatasetKey]struct{}{}
	for index, dataset := range value.Datasets {
		if !validKey(string(dataset.Key)) || len(dataset.Categories) == 0 || len(dataset.Operations) == 0 {
			return OwnerDefinition{}, invalid("datasets", fmt.Sprintf("item %d is invalid", index))
		}
		if _, exists := seen[dataset.Key]; exists {
			return OwnerDefinition{}, invalid("datasets", fmt.Sprintf("contains duplicate %q", dataset.Key))
		}
		seen[dataset.Key] = struct{}{}
		for _, category := range dataset.Categories {
			if _, exists := catalog.categories[category]; !exists {
				return OwnerDefinition{}, invalid("datasets", fmt.Sprintf("%q references unknown category %q", dataset.Key, category))
			}
		}
		for _, operation := range dataset.Operations {
			if !validRightsOperation(operation) {
				return OwnerDefinition{}, invalid("datasets", fmt.Sprintf("%q has invalid operation", dataset.Key))
			}
		}
		for _, rule := range dataset.RetentionRules {
			if _, exists := catalog.retention[rule]; !exists {
				return OwnerDefinition{}, invalid("datasets", fmt.Sprintf("%q references unknown retention rule", dataset.Key))
			}
		}
	}
	canonicalizeOwner(&value)
	copy := value
	copy.Ref.Digest = ""
	encoded, _ := json.Marshal(copy)
	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	if value.Ref.Digest != "" && value.Ref.Digest != digest {
		return OwnerDefinition{}, &Error{Kind: ErrorDefinitionDrift, Field: "owner.digest", Message: "does not match owner definition"}
	}
	value.Ref.Digest = digest
	return value, nil
}

func canonicalizeDefinition(value *Definition) {
	for index := range value.Notices {
		slices.SortFunc(value.Notices[index].Purposes, func(a, b PurposeRef) int {
			return compareRefs(string(a.Key), a.Revision, string(b.Key), b.Revision)
		})
	}
	for index := range value.Purposes {
		purpose := &value.Purposes[index]
		slices.Sort(purpose.Categories)
		slices.SortFunc(purpose.Notices, func(a, b NoticeRef) int {
			return compareRefs(string(a.Key), a.Revision, string(b.Key), b.Revision)
		})
		slices.SortFunc(purpose.Restrictions, func(a, b RestrictionDefinition) int {
			return strings.Compare(string(a.Key), string(b.Key))
		})
		for ruleIndex := range purpose.SignalRules {
			slices.Sort(purpose.SignalRules[ruleIndex].Restrictions)
		}
		slices.SortFunc(purpose.SignalRules, func(a, b SignalRule) int {
			return strings.Compare(string(a.Signal)+"\x00"+string(a.Effect), string(b.Signal)+"\x00"+string(b.Effect))
		})
	}
	for index := range value.RetentionRules {
		slices.Sort(value.RetentionRules[index].Categories)
	}
	if value.Owner != nil {
		canonicalizeOwner(value.Owner)
	}
	if value.Coordination != nil {
		for index := range value.Coordination.Owners {
			canonicalizeOwner(&value.Coordination.Owners[index])
		}
		slices.SortFunc(value.Coordination.Owners, func(a, b OwnerDefinition) int {
			return strings.Compare(string(a.Ref.Key), string(b.Ref.Key))
		})
		slices.SortFunc(value.Coordination.RightsPolicies, func(a, b RightsPolicy) int {
			return strings.Compare(string(a.Operation), string(b.Operation))
		})
	}
	slices.SortFunc(value.SubjectKinds, func(a, b SubjectKindDefinition) int { return strings.Compare(string(a.Key), string(b.Key)) })
	slices.SortFunc(value.DataCategories, func(a, b DataCategoryDefinition) int { return strings.Compare(string(a.Key), string(b.Key)) })
	slices.SortFunc(value.Notices, func(a, b NoticeDefinition) int {
		return compareRefs(string(a.Ref.Key), a.Ref.Revision, string(b.Ref.Key), b.Ref.Revision)
	})
	slices.SortFunc(value.Purposes, func(a, b PurposeDefinition) int {
		return compareRefs(string(a.Ref.Key), a.Ref.Revision, string(b.Ref.Key), b.Ref.Revision)
	})
	slices.SortFunc(value.ActivePurposes, func(a, b ActivePurpose) int { return strings.Compare(string(a.Key), string(b.Key)) })
	slices.SortFunc(value.Signals, func(a, b SignalDefinition) int { return strings.Compare(string(a.Key), string(b.Key)) })
	slices.SortFunc(value.RetentionRules, func(a, b RetentionRuleDefinition) int {
		return compareRefs(string(a.Ref.Key), a.Ref.Revision, string(b.Ref.Key), b.Ref.Revision)
	})
}

func canonicalizeOwner(value *OwnerDefinition) {
	if value == nil {
		return
	}
	slices.Sort(value.SubjectKinds)
	for index := range value.Datasets {
		slices.Sort(value.Datasets[index].Categories)
		slices.Sort(value.Datasets[index].Operations)
		slices.SortFunc(value.Datasets[index].RetentionRules, func(a, b RetentionRuleRef) int {
			return compareRefs(string(a.Key), a.Revision, string(b.Key), b.Revision)
		})
	}
	slices.SortFunc(value.Datasets, func(a, b DatasetDefinition) int {
		return strings.Compare(string(a.Key), string(b.Key))
	})
}

func compareRefs(a string, ar Revision, b string, br Revision) int {
	if result := strings.Compare(a, b); result != 0 {
		return result
	}
	if ar < br {
		return -1
	}
	if ar > br {
		return 1
	}
	return 0
}

func validKey(value string) bool            { return definitionKeyPattern.MatchString(value) }
func validPurposeRef(value PurposeRef) bool { return validKey(string(value.Key)) && value.Revision > 0 }
func validNoticeRef(value NoticeRef) bool   { return validKey(string(value.Key)) && value.Revision > 0 }
func validRetentionRef(value RetentionRuleRef) bool {
	return validKey(string(value.Key)) && value.Revision > 0
}
func validPeriod(value CalendarPeriod) bool {
	return value.Years >= 0 && value.Months >= 0 && value.Months <= 120 && value.Days >= 0 && value.Days <= 3660 &&
		(value.Years > 0 || value.Months > 0 || value.Days > 0)
}
func validBasis(value ProcessingBasis) bool {
	switch value {
	case BasisConsent, BasisContract, BasisLegalObligation, BasisVitalInterests, BasisPublicTask, BasisLegitimateInterest:
		return true
	default:
		return false
	}
}
func validRightsOperation(value RightsOperation) bool {
	switch value {
	case RightAccess, RightPortability, RightRectification, RightErasure, RightRestriction, RightObjection, RetentionReview:
		return true
	default:
		return false
	}
}
func validReviewOutcome(value ReviewOutcome) bool {
	return value == ReviewOwnerDecision || value == ReviewAnonymize || value == ReviewDelete
}

func (catalog *Catalog) Version() uint64 {
	if catalog == nil {
		return 0
	}
	return catalog.version
}
func (catalog *Catalog) Consumer() string {
	if catalog == nil {
		return ""
	}
	return catalog.consumer
}
func (catalog *Catalog) Digest() string {
	if catalog == nil {
		return ""
	}
	return catalog.digest
}
func (catalog *Catalog) Limits() Limits {
	if catalog == nil {
		return Limits{}
	}
	return catalog.limits
}

func (catalog *Catalog) Owner() (OwnerDefinition, bool) {
	if catalog == nil || catalog.owner == nil {
		return OwnerDefinition{}, false
	}
	return *catalog.owner, true
}

func (catalog *Catalog) Owners() []OwnerDefinition {
	if catalog == nil {
		return nil
	}
	result := make([]OwnerDefinition, 0, len(catalog.owners))
	for _, owner := range catalog.owners {
		result = append(result, owner)
	}
	slices.SortFunc(result, func(a, b OwnerDefinition) int {
		return strings.Compare(string(a.Ref.Key), string(b.Ref.Key))
	})
	return result
}
