package authorization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var (
	qualifiedKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	slugKeyPattern      = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// Catalog is an immutable, validated consumer authorization declaration.
type Catalog struct {
	consumer       ConsumerKey
	version        uint
	digest         string
	capabilities   map[CapabilityKey]CapabilityDefinition
	roles          map[RoleKey]RoleDefinition
	accessLayers   map[AccessLayerKey]AccessLayerDefinition
	scopeChildren  map[ScopeType]map[ScopeType]struct{}
	scopeTypes     map[ScopeType]ScopeTypeDefinition
	rootScope      ScopeType
	protectedRole  RoleKey
	constraints    map[ConstraintKey]ConstraintDefinition
	automaticRules map[string]AutomaticRuleDefinition
}

// Compile validates and copies a complete consumer Definition.
func Compile(definition Definition) (*Catalog, error) {
	if !slugKeyPattern.MatchString(string(definition.Consumer)) {
		return nil, invalidDefinition("consumer", "must be a lowercase slug")
	}
	if definition.Version == 0 {
		return nil, invalidDefinition("version", "must be greater than zero")
	}

	catalog := &Catalog{
		consumer:       definition.Consumer,
		version:        definition.Version,
		capabilities:   make(map[CapabilityKey]CapabilityDefinition),
		roles:          make(map[RoleKey]RoleDefinition),
		accessLayers:   make(map[AccessLayerKey]AccessLayerDefinition),
		scopeChildren:  make(map[ScopeType]map[ScopeType]struct{}),
		scopeTypes:     make(map[ScopeType]ScopeTypeDefinition),
		constraints:    make(map[ConstraintKey]ConstraintDefinition),
		automaticRules: make(map[string]AutomaticRuleDefinition),
	}
	for _, capability := range coreCapabilities() {
		catalog.capabilities[capability.Key] = capability
	}
	for index, capability := range definition.Capabilities {
		field := "capabilities[" + itoa(index) + "]"
		if err := catalog.addCapability(field, capability); err != nil {
			return nil, err
		}
	}
	if err := catalog.compileScopes(definition.Scopes); err != nil {
		return nil, err
	}
	if err := catalog.validateCapabilities(); err != nil {
		return nil, err
	}
	if err := catalog.compileAccessLayers(definition.AccessLayers); err != nil {
		return nil, err
	}
	if err := catalog.compileRoles(definition.Roles); err != nil {
		return nil, err
	}
	if err := catalog.compileConstraints(definition.Constraints); err != nil {
		return nil, err
	}
	if err := catalog.compileAutomatic(definition.Automatic); err != nil {
		return nil, err
	}
	digest, err := catalogDigest(catalog)
	if err != nil {
		return nil, invalidDefinition("definition", "cannot compute digest: %v", err)
	}
	catalog.digest = digest
	return catalog, nil
}

// MustCompile is Compile for static consumer definitions. It panics when the
// declaration is invalid and should only be used during application assembly.
func MustCompile(definition Definition) *Catalog {
	catalog, err := Compile(definition)
	if err != nil {
		panic(err)
	}
	return catalog
}

func coreCapabilities() []CapabilityDefinition {
	return []CapabilityDefinition{
		{
			Key: CapabilityManage, Version: 1, Binding: BindingNormal,
			Risk: RiskHigh, Audit: AuditFull, Delegable: true,
		},
		{Key: CapabilityAuditRead, Version: 1, Binding: BindingProtectedOnly, Risk: RiskHigh, Audit: AuditFull},
		{Key: CapabilityApplicationCreate, Version: 1, Binding: BindingAccessLayerEligible},
		{Key: CapabilityApplicationReadOwn, Version: 1, Binding: BindingAccessLayerEligible},
		{Key: CapabilityApplicationWithdraw, Version: 1, Binding: BindingAccessLayerEligible},
		{Key: CapabilityInvitationAccept, Version: 1, Binding: BindingAccessLayerEligible},
	}
}

func (catalog *Catalog) addCapability(field string, capability CapabilityDefinition) error {
	if !qualifiedKeyPattern.MatchString(string(capability.Key)) {
		return invalidDefinition(field+".key", "must be a qualified lowercase key")
	}
	if capability.Version == 0 {
		return invalidDefinition(field+".version", "must be greater than zero")
	}
	if capability.Binding == "" {
		capability.Binding = BindingNormal
	}
	if capability.Binding != BindingNormal && capability.Binding != BindingProtectedOnly && capability.Binding != BindingAccessLayerEligible {
		return invalidDefinition(field+".binding", "unknown binding class %q", capability.Binding)
	}
	if capability.QueryableRelation != "" && !slugKeyPattern.MatchString(string(capability.QueryableRelation)) {
		return invalidDefinition(field+".queryable_relation", "must be a lowercase slug")
	}
	if _, exists := catalog.capabilities[capability.Key]; exists {
		return invalidDefinition(field+".key", "duplicate or reserved capability %q", capability.Key)
	}
	capability.AllowedScopes = slices.Clone(capability.AllowedScopes)
	capability.EligibleSubjects = slices.Clone(capability.EligibleSubjects)
	catalog.capabilities[capability.Key] = capability
	return nil
}

func (catalog *Catalog) compileScopes(schema ScopeSchema) error {
	rootCount := 0
	for index, scopeType := range schema.Types {
		field := "scopes.types[" + itoa(index) + "]"
		if !slugKeyPattern.MatchString(string(scopeType.Key)) {
			return invalidDefinition(field+".key", "must be a lowercase slug")
		}
		if _, exists := catalog.scopeTypes[scopeType.Key]; exists {
			return invalidDefinition(field+".key", "duplicate scope type %q", scopeType.Key)
		}
		scopeType.Children = slices.Clone(scopeType.Children)
		catalog.scopeTypes[scopeType.Key] = scopeType
		if scopeType.Root {
			rootCount++
			catalog.rootScope = scopeType.Key
		}
	}
	if rootCount != 1 {
		return invalidDefinition("scopes", "must declare exactly one root scope type")
	}
	for parent, scopeType := range catalog.scopeTypes {
		children := make(map[ScopeType]struct{}, len(scopeType.Children))
		for _, child := range scopeType.Children {
			if child == parent {
				return invalidDefinition("scopes", "scope type %q cannot contain itself", parent)
			}
			if _, exists := catalog.scopeTypes[child]; !exists {
				return invalidDefinition("scopes", "scope type %q references unknown child %q", parent, child)
			}
			if _, duplicate := children[child]; duplicate {
				return invalidDefinition("scopes", "scope type %q repeats child %q", parent, child)
			}
			children[child] = struct{}{}
		}
		catalog.scopeChildren[parent] = children
	}
	if catalog.scopeTypeGraphHasCycle() {
		return invalidDefinition("scopes", "scope type graph must be acyclic")
	}
	reachable := make(map[ScopeType]struct{}, len(catalog.scopeTypes))
	var walk func(ScopeType)
	walk = func(current ScopeType) {
		if _, seen := reachable[current]; seen {
			return
		}
		reachable[current] = struct{}{}
		for child := range catalog.scopeChildren[current] {
			walk(child)
		}
	}
	walk(catalog.rootScope)
	if len(reachable) != len(catalog.scopeTypes) {
		return invalidDefinition("scopes", "every scope type must be reachable from the root")
	}
	return nil
}

func (catalog *Catalog) validateCapabilities() error {
	validSubjectKinds := map[SubjectKind]bool{
		SubjectAnonymous: true,
		SubjectGuest:     true,
		SubjectUser:      true,
		SubjectService:   true,
	}
	for key, capability := range catalog.capabilities {
		seenScopes := make(map[ScopeType]struct{}, len(capability.AllowedScopes))
		for _, scopeType := range capability.AllowedScopes {
			if _, exists := catalog.scopeTypes[scopeType]; !exists {
				return invalidDefinition("capabilities", "capability %q references unknown scope type %q", key, scopeType)
			}
			if _, duplicate := seenScopes[scopeType]; duplicate {
				return invalidDefinition("capabilities", "capability %q repeats scope type %q", key, scopeType)
			}
			seenScopes[scopeType] = struct{}{}
		}
		seenSubjects := make(map[SubjectKind]struct{}, len(capability.EligibleSubjects))
		for _, kind := range capability.EligibleSubjects {
			if !validSubjectKinds[kind] {
				return invalidDefinition("capabilities", "capability %q has unknown eligible subject %q", key, kind)
			}
			if _, duplicate := seenSubjects[kind]; duplicate {
				return invalidDefinition("capabilities", "capability %q repeats eligible subject %q", key, kind)
			}
			seenSubjects[kind] = struct{}{}
		}
	}
	return nil
}

func (catalog *Catalog) scopeTypeGraphHasCycle() bool {
	const (
		unseen = iota
		visiting
		done
	)
	state := make(map[ScopeType]int, len(catalog.scopeTypes))
	var visit func(ScopeType) bool
	visit = func(current ScopeType) bool {
		if state[current] == visiting {
			return true
		}
		if state[current] == done {
			return false
		}
		state[current] = visiting
		for child := range catalog.scopeChildren[current] {
			if visit(child) {
				return true
			}
		}
		state[current] = done
		return false
	}
	for scopeType := range catalog.scopeTypes {
		if visit(scopeType) {
			return true
		}
	}
	return false
}

func (catalog *Catalog) compileAccessLayers(definitions []AccessLayerDefinition) error {
	for index, layer := range definitions {
		field := "access_layers[" + itoa(index) + "]"
		if layer.Key != AccessLayerVisitor && layer.Key != AccessLayerAuthenticated {
			return invalidDefinition(field+".key", "unknown access layer %q", layer.Key)
		}
		if _, exists := catalog.accessLayers[layer.Key]; exists {
			return invalidDefinition(field+".key", "duplicate access layer %q", layer.Key)
		}
		layer.Capabilities = slices.Clone(layer.Capabilities)
		for _, capabilityKey := range layer.Capabilities {
			capability, exists := catalog.capabilities[capabilityKey]
			if !exists {
				return invalidDefinition(field+".capabilities", "unknown capability %q", capabilityKey)
			}
			if capability.Binding != BindingAccessLayerEligible {
				return invalidDefinition(field+".capabilities", "capability %q cannot bind to an access layer", capabilityKey)
			}
		}
		catalog.accessLayers[layer.Key] = layer
	}
	for _, required := range []AccessLayerKey{AccessLayerVisitor, AccessLayerAuthenticated} {
		if _, exists := catalog.accessLayers[required]; !exists {
			return invalidDefinition("access_layers", "missing required access layer %q", required)
		}
	}
	return nil
}

func (catalog *Catalog) compileRoles(definitions []RoleDefinition) error {
	protected := 0
	for index, role := range definitions {
		field := "roles[" + itoa(index) + "]"
		if !slugKeyPattern.MatchString(string(role.Key)) {
			return invalidDefinition(field+".key", "must be a lowercase slug")
		}
		if strings.TrimSpace(role.DisplayName) == "" {
			return invalidDefinition(field+".display_name", "must not be empty")
		}
		if _, exists := catalog.roles[role.Key]; exists {
			return invalidDefinition(field+".key", "duplicate role %q", role.Key)
		}
		role.Capabilities = slices.Clone(role.Capabilities)
		role.Assignment.Sources = slices.Clone(role.Assignment.Sources)
		hasManage := false
		seenCapabilities := make(map[CapabilityKey]struct{}, len(role.Capabilities))
		for _, capabilityKey := range role.Capabilities {
			if _, duplicate := seenCapabilities[capabilityKey]; duplicate {
				return invalidDefinition(field+".capabilities", "duplicate capability %q", capabilityKey)
			}
			seenCapabilities[capabilityKey] = struct{}{}
			capability, exists := catalog.capabilities[capabilityKey]
			if !exists {
				return invalidDefinition(field+".capabilities", "unknown capability %q", capabilityKey)
			}
			if capability.Binding == BindingAccessLayerEligible {
				return invalidDefinition(field+".capabilities", "capability %q can only bind to an access layer", capabilityKey)
			}
			if capability.Binding == BindingProtectedOnly && !role.Protected {
				return invalidDefinition(field+".capabilities", "capability %q requires the protected role", capabilityKey)
			}
			hasManage = hasManage || capabilityKey == CapabilityManage
		}
		seenSources := make(map[GrantSource]struct{}, len(role.Assignment.Sources))
		for _, source := range role.Assignment.Sources {
			if !validGrantSource(source) {
				return invalidDefinition(field+".assignment.sources", "unknown source %q", source)
			}
			if _, duplicate := seenSources[source]; duplicate {
				return invalidDefinition(field+".assignment.sources", "duplicate source %q", source)
			}
			seenSources[source] = struct{}{}
		}
		if role.Protected {
			protected++
			catalog.protectedRole = role.Key
			if !hasManage {
				return invalidDefinition(field+".capabilities", "protected role must include %q", CapabilityManage)
			}
			if len(role.Assignment.Sources) != 0 {
				return invalidDefinition(field+".assignment", "protected role assignment sources are fixed by the module")
			}
		}
		catalog.roles[role.Key] = role
	}
	if protected != 1 {
		return invalidDefinition("roles", "must declare exactly one protected role")
	}
	return nil
}

func (catalog *Catalog) compileConstraints(definitions []ConstraintDefinition) error {
	for index, constraint := range definitions {
		field := "constraints[" + itoa(index) + "]"
		if !qualifiedKeyPattern.MatchString(string(constraint.Key)) {
			return invalidDefinition(field+".key", "must be a qualified lowercase key")
		}
		if constraint.Version == 0 {
			return invalidDefinition(field+".version", "must be greater than zero")
		}
		if constraint.Mode != ConstraintGlobal && constraint.Mode != ConstraintSource {
			return invalidDefinition(field+".mode", "must be global or source")
		}
		if len(constraint.Capabilities) == 0 {
			return invalidDefinition(field+".capabilities", "must not be empty")
		}
		constraint.Capabilities = slices.Clone(constraint.Capabilities)
		constraint.Roles = slices.Clone(constraint.Roles)
		constraint.AccessLayers = slices.Clone(constraint.AccessLayers)
		for _, capability := range constraint.Capabilities {
			if _, exists := catalog.capabilities[capability]; !exists {
				return invalidDefinition(field+".capabilities", "unknown capability %q", capability)
			}
		}
		if constraint.Mode == ConstraintGlobal &&
			(constraint.AllNormalRoles || len(constraint.Roles) != 0 || len(constraint.AccessLayers) != 0) {
			return invalidDefinition(field, "global constraint cannot select policy sources")
		}
		if constraint.Mode == ConstraintSource && !constraint.AllNormalRoles &&
			len(constraint.Roles) == 0 && len(constraint.AccessLayers) == 0 {
			return invalidDefinition(field, "source constraint must select at least one role or access layer")
		}
		for _, role := range constraint.Roles {
			if _, exists := catalog.roles[role]; !exists {
				return invalidDefinition(field+".roles", "unknown role %q", role)
			}
		}
		for _, layer := range constraint.AccessLayers {
			if _, exists := catalog.accessLayers[layer]; !exists {
				return invalidDefinition(field+".access_layers", "unknown access layer %q", layer)
			}
		}
		if _, exists := catalog.constraints[constraint.Key]; exists {
			return invalidDefinition(field+".key", "duplicate constraint %q", constraint.Key)
		}
		catalog.constraints[constraint.Key] = constraint
	}
	return nil
}

func (catalog *Catalog) compileAutomatic(definitions []AutomaticRuleDefinition) error {
	for index, rule := range definitions {
		field := "automatic[" + itoa(index) + "]"
		if !qualifiedKeyPattern.MatchString(rule.Key) {
			return invalidDefinition(field+".key", "must be a qualified lowercase key")
		}
		if !qualifiedKeyPattern.MatchString(string(rule.Trigger)) || !qualifiedKeyPattern.MatchString(string(rule.Predicate)) {
			return invalidDefinition(field, "trigger and predicate must be qualified lowercase keys")
		}
		role, exists := catalog.roles[rule.Role]
		if !exists {
			return invalidDefinition(field+".role", "unknown role %q", rule.Role)
		}
		if role.Protected || !slices.Contains(role.Assignment.Sources, GrantSourceAutomatic) {
			return invalidDefinition(field+".role", "role %q does not allow automatic assignment", rule.Role)
		}
		if _, exists := catalog.automaticRules[rule.Key]; exists {
			return invalidDefinition(field+".key", "duplicate automatic rule %q", rule.Key)
		}
		catalog.automaticRules[rule.Key] = rule
	}
	return nil
}

func catalogDigest(catalog *Catalog) (string, error) {
	canonical := Definition{Consumer: catalog.consumer, Version: catalog.version}
	for _, capability := range catalog.capabilities {
		capability.AllowedScopes = slices.Clone(capability.AllowedScopes)
		capability.EligibleSubjects = slices.Clone(capability.EligibleSubjects)
		slices.Sort(capability.AllowedScopes)
		slices.Sort(capability.EligibleSubjects)
		canonical.Capabilities = append(canonical.Capabilities, capability)
	}
	for _, scopeType := range catalog.scopeTypes {
		scopeType.Children = slices.Clone(scopeType.Children)
		slices.Sort(scopeType.Children)
		canonical.Scopes.Types = append(canonical.Scopes.Types, scopeType)
	}
	for _, layer := range catalog.accessLayers {
		layer.Capabilities = slices.Clone(layer.Capabilities)
		slices.Sort(layer.Capabilities)
		canonical.AccessLayers = append(canonical.AccessLayers, layer)
	}
	for _, role := range catalog.roles {
		role.Capabilities = slices.Clone(role.Capabilities)
		role.Assignment.Sources = slices.Clone(role.Assignment.Sources)
		slices.Sort(role.Capabilities)
		slices.Sort(role.Assignment.Sources)
		canonical.Roles = append(canonical.Roles, role)
	}
	for _, constraint := range catalog.constraints {
		constraint.Capabilities = slices.Clone(constraint.Capabilities)
		constraint.Roles = slices.Clone(constraint.Roles)
		constraint.AccessLayers = slices.Clone(constraint.AccessLayers)
		slices.Sort(constraint.Capabilities)
		slices.Sort(constraint.Roles)
		slices.Sort(constraint.AccessLayers)
		canonical.Constraints = append(canonical.Constraints, constraint)
	}
	for _, rule := range catalog.automaticRules {
		canonical.Automatic = append(canonical.Automatic, rule)
	}
	sort.Slice(canonical.Capabilities, func(i, j int) bool {
		return canonical.Capabilities[i].Key < canonical.Capabilities[j].Key
	})
	sort.Slice(canonical.Scopes.Types, func(i, j int) bool {
		return canonical.Scopes.Types[i].Key < canonical.Scopes.Types[j].Key
	})
	sort.Slice(canonical.Roles, func(i, j int) bool { return canonical.Roles[i].Key < canonical.Roles[j].Key })
	sort.Slice(canonical.AccessLayers, func(i, j int) bool {
		return canonical.AccessLayers[i].Key < canonical.AccessLayers[j].Key
	})
	sort.Slice(canonical.Constraints, func(i, j int) bool {
		return canonical.Constraints[i].Key < canonical.Constraints[j].Key
	})
	sort.Slice(canonical.Automatic, func(i, j int) bool {
		return canonical.Automatic[i].Key < canonical.Automatic[j].Key
	})
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validGrantSource(source GrantSource) bool {
	switch source {
	case GrantSourceAutomatic, GrantSourceApplication, GrantSourceInvitation,
		GrantSourceDirect, GrantSourceGroup, GrantSourceServiceProvisioning,
		GrantSourceImportSync, GrantSourceBootstrap, GrantSourceRecovery,
		GrantSourceInitialClaim:
		return true
	default:
		return false
	}
}

func itoa(value int) string {
	const digits = "0123456789"
	if value < 10 {
		return string(digits[value])
	}
	result := ""
	for value > 0 {
		result = string(digits[value%10]) + result
		value /= 10
	}
	return result
}

func (catalog *Catalog) Consumer() ConsumerKey {
	if catalog == nil {
		return ""
	}
	return catalog.consumer
}

func (catalog *Catalog) Version() uint {
	if catalog == nil {
		return 0
	}
	return catalog.version
}

func (catalog *Catalog) Digest() string {
	if catalog == nil {
		return ""
	}
	return catalog.digest
}

func (catalog *Catalog) ProtectedRole() RoleKey {
	if catalog == nil {
		return ""
	}
	return catalog.protectedRole
}

func (catalog *Catalog) RootScopeType() ScopeType {
	if catalog == nil {
		return ""
	}
	return catalog.rootScope
}

func (catalog *Catalog) HasCapability(key CapabilityKey) bool {
	if catalog == nil {
		return false
	}
	_, exists := catalog.capabilities[key]
	return exists
}

func (catalog *Catalog) AllowsScopeChild(parent, child ScopeType) bool {
	if catalog == nil {
		return false
	}
	_, exists := catalog.scopeChildren[parent][child]
	return exists
}
