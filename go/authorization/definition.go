package authorization

// Definition is the complete code-owned declaration compiled at consumer
// startup. Compile copies every slice before returning a Catalog.
type Definition struct {
	Consumer     ConsumerKey
	Version      uint
	Capabilities []CapabilityDefinition
	Scopes       ScopeSchema
	AccessLayers []AccessLayerDefinition
	Roles        []RoleDefinition
	Constraints  []ConstraintDefinition
	Automatic    []AutomaticRuleDefinition
}

type CapabilityDefinition struct {
	Key               CapabilityKey
	Version           uint
	Binding           BindingClass
	Risk              RiskLevel
	Audit             AuditMode
	AllowedScopes     []ScopeType
	EligibleSubjects  []SubjectKind
	QueryableRelation RelationKind
	Delegable         bool
}

type ScopeSchema struct {
	Types []ScopeTypeDefinition
}

type ScopeTypeDefinition struct {
	Key      ScopeType
	Root     bool
	Children []ScopeType
}

type AccessLayerDefinition struct {
	Key          AccessLayerKey
	Capabilities []CapabilityKey
}

type RoleDefinition struct {
	Key          RoleKey
	DisplayName  string
	Protected    bool
	Capabilities []CapabilityKey
	Assignment   AssignmentPolicy
}

// ConstraintDefinition identifies a code-owned constraint. Evaluation is
// attached when constructing an Engine and is not part of the Catalog digest.
type ConstraintDefinition struct {
	Key          ConstraintKey
	Version      uint
	Mode         ConstraintMode
	Capabilities []CapabilityKey
	// AllNormalRoles also selects custom roles created after startup while
	// deliberately excluding the protected administrator role.
	AllNormalRoles bool
	Roles          []RoleKey
	AccessLayers   []AccessLayerKey
}

type AutomaticRuleDefinition struct {
	Key       string
	Trigger   TriggerKey
	Predicate PredicateKey
	Role      RoleKey
	Enabled   bool
}
