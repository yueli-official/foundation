package capability

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var (
	capabilityKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9_-]*)+$`)
	identifierPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
)

// Snapshot is the immutable list/get surface consumed by HTTP adapters and
// tests. Construction validates the whole contract and derives effective
// state, so list and get cannot drift.
type Snapshot struct {
	manifest     Manifest
	capabilities map[string]Capability
	providers    map[string]Provider
}

func NewSnapshot(input Manifest) (*Snapshot, error) {
	value := cloneManifest(input)
	if value.APIVersion != "" && value.APIVersion != APIVersion {
		return nil, contractError(ErrorUnsupported, "capability manifest apiVersion %q is unsupported", value.APIVersion)
	}
	if value.Kind != "" && value.Kind != Kind {
		return nil, contractError(ErrorUnsupported, "capability manifest kind %q is unsupported", value.Kind)
	}
	value.APIVersion = APIVersion
	value.Kind = Kind
	if err := validateService(value.Service); err != nil {
		return nil, err
	}
	if value.GeneratedAt.IsZero() {
		return nil, contractError(ErrorRequired, "capability manifest generatedAt is required")
	}
	if strings.TrimSpace(value.Redaction.Policy) == "" || strings.TrimSpace(value.Redaction.Version) == "" {
		return nil, contractError(ErrorRequired, "capability manifest redaction policy and version are required")
	}
	if len(value.Capabilities) == 0 {
		return nil, contractError(ErrorRequired, "capability manifest requires at least one capability")
	}

	capabilities := make(map[string]Capability, len(value.Capabilities))
	for index := range value.Capabilities {
		item := &value.Capabilities[index]
		if err := normalizeCapability(item); err != nil {
			return nil, fmt.Errorf("capability %q: %w", item.Key, err)
		}
		if _, exists := capabilities[item.Key]; exists {
			return nil, contractError(ErrorDuplicate, "duplicate capability %q", item.Key)
		}
		capabilities[item.Key] = cloneCapability(*item)
	}
	sort.Slice(value.Capabilities, func(i, j int) bool { return value.Capabilities[i].Key < value.Capabilities[j].Key })

	providers := make(map[string]Provider, len(value.Providers))
	for index := range value.Providers {
		item := &value.Providers[index]
		if err := normalizeProvider(item, capabilities); err != nil {
			return nil, fmt.Errorf("provider %q: %w", item.Key, err)
		}
		if _, exists := providers[item.Key]; exists {
			return nil, contractError(ErrorDuplicate, "duplicate provider %q", item.Key)
		}
		providers[item.Key] = cloneProvider(*item)
	}
	sort.Slice(value.Providers, func(i, j int) bool { return value.Providers[i].Key < value.Providers[j].Key })
	for index := range value.Capabilities {
		item := &value.Capabilities[index]
		if item.ProviderInstance != "" {
			provider, ok := providers[item.ProviderInstance]
			if !ok {
				return nil, contractError(ErrorUnknownReference, "capability %q references unknown provider instance %q", item.Key, item.ProviderInstance)
			}
			if !contains(provider.CapabilityKeys, item.Key) {
				return nil, contractError(ErrorReferenceMismatch, "capability %q provider instance %q does not provide it", item.Key, item.ProviderInstance)
			}
			if !provider.Registered {
				return nil, contractError(ErrorReferenceMismatch, "capability %q provider instance %q is not registered", item.Key, item.ProviderInstance)
			}
			if item.Adapter != "" && item.Adapter != provider.Adapter {
				return nil, contractError(ErrorReferenceMismatch, "capability %q adapter %q does not match provider instance adapter %q", item.Key, item.Adapter, provider.Adapter)
			}
			item.Adapter = provider.Adapter
		}
		capabilities[item.Key] = cloneCapability(*item)
	}
	if err := normalizeLinks(&value.Links); err != nil {
		return nil, fmt.Errorf("manifest links: %w", err)
	}

	return &Snapshot{manifest: value, capabilities: capabilities, providers: providers}, nil
}

func (snapshot *Snapshot) Manifest() Manifest {
	if snapshot == nil {
		return Manifest{}
	}
	return cloneManifest(snapshot.manifest)
}

func (snapshot *Snapshot) ListCapabilities() []Capability {
	if snapshot == nil {
		return nil
	}
	items := make([]Capability, len(snapshot.manifest.Capabilities))
	for index, item := range snapshot.manifest.Capabilities {
		items[index] = cloneCapability(item)
	}
	return items
}

func (snapshot *Snapshot) Capability(key string) (Capability, bool) {
	if snapshot == nil {
		return Capability{}, false
	}
	item, ok := snapshot.capabilities[strings.TrimSpace(key)]
	return cloneCapability(item), ok
}

func (snapshot *Snapshot) ListProviders() []Provider {
	if snapshot == nil {
		return nil
	}
	items := make([]Provider, len(snapshot.manifest.Providers))
	for index, item := range snapshot.manifest.Providers {
		items[index] = cloneProvider(item)
	}
	return items
}

func (snapshot *Snapshot) Provider(key string) (Provider, bool) {
	if snapshot == nil {
		return Provider{}, false
	}
	item, ok := snapshot.providers[strings.TrimSpace(key)]
	return cloneProvider(item), ok
}

func validateService(value ServiceMetadata) error {
	for _, required := range []struct {
		name  string
		value string
	}{
		{name: "name", value: value.Name},
		{name: "version", value: value.Version},
		{name: "buildSha", value: value.BuildSHA},
		{name: "deployment", value: value.Deployment},
	} {
		if strings.TrimSpace(required.value) == "" {
			return contractError(ErrorRequired, "service.%s is required", required.name)
		}
	}
	if !identifierPattern.MatchString(value.Name) {
		return contractError(ErrorInvalid, "service.name %q is not a canonical key", value.Name)
	}
	return nil
}

func normalizeCapability(item *Capability) error {
	if !capabilityKeyPattern.MatchString(item.Key) {
		return contractError(ErrorInvalid, "key is not canonical")
	}
	if strings.TrimSpace(item.ContractVersion) == "" {
		return contractError(ErrorRequired, "contractVersion is required")
	}
	if !validSupport(item.Support) {
		return contractError(ErrorInvalid, "support %q is invalid", item.Support)
	}
	return normalizeRuntimeState(runtimeState{support: item.Support, configuration: &item.Configuration, enablement: &item.Enablement, health: &item.Health, effective: &item.Effective, operations: &item.Operations, requiredConfig: &item.RequiredConfig, links: &item.Links})
}

func normalizeProvider(item *Provider, capabilities map[string]Capability) error {
	if !identifierPattern.MatchString(item.Key) {
		return contractError(ErrorInvalid, "key is not canonical")
	}
	if !identifierPattern.MatchString(item.Adapter) {
		return contractError(ErrorInvalid, "adapter is required and must be canonical")
	}
	if len(item.CapabilityKeys) == 0 {
		return contractError(ErrorRequired, "at least one capability key is required")
	}
	if err := normalizeStrings(&item.CapabilityKeys, "capability key"); err != nil {
		return err
	}
	for _, key := range item.CapabilityKeys {
		capability, ok := capabilities[key]
		if !ok {
			return contractError(ErrorUnknownReference, "references unknown capability %q", key)
		}
		if capability.Support != SupportSupported {
			return contractError(ErrorReferenceMismatch, "references unsupported capability %q", key)
		}
	}
	support := SupportUnsupported
	if item.Registered {
		support = SupportSupported
	}
	return normalizeRuntimeState(runtimeState{support: support, configuration: &item.Configuration, enablement: &item.Enablement, health: &item.Health, effective: &item.Effective, operations: &item.Operations, requiredConfig: &item.RequiredConfig, links: &item.Links})
}

type runtimeState struct {
	support        Support
	configuration  *Configuration
	enablement     *Enablement
	health         *Health
	effective      *bool
	operations     *[]string
	requiredConfig *[]ConfigField
	links          *[]Link
}

func normalizeRuntimeState(state runtimeState) error {
	if !validConfiguration(*state.configuration) {
		return contractError(ErrorInvalid, "configuration %q is invalid", *state.configuration)
	}
	if !validEnablement(*state.enablement) {
		return contractError(ErrorInvalid, "enablement %q is invalid", *state.enablement)
	}
	if !validHealth(*state.health) {
		return contractError(ErrorInvalid, "health %q is invalid", *state.health)
	}
	if err := normalizeOperations(state.operations); err != nil {
		return err
	}
	if err := normalizeConfig(state.requiredConfig); err != nil {
		return err
	}
	if len(*state.requiredConfig) > 0 {
		*state.configuration = configurationFrom(*state.requiredConfig)
	}
	if err := normalizeLinks(state.links); err != nil {
		return err
	}
	*state.effective = effective(state.support, *state.configuration, *state.enablement, *state.health)
	return nil
}

func normalizeOperations(values *[]string) error {
	return normalizeStrings(values, "operation")
}

func normalizeStrings(values *[]string, label string) error {
	if *values == nil {
		*values = []string{}
	}
	seen := map[string]bool{}
	for index, value := range *values {
		value = strings.TrimSpace(value)
		if value == "" {
			return contractError(ErrorRequired, "%s is required", label)
		}
		if seen[value] {
			return contractError(ErrorDuplicate, "duplicate %s %q", label, value)
		}
		seen[value] = true
		(*values)[index] = value
	}
	sort.Strings(*values)
	return nil
}

func normalizeConfig(values *[]ConfigField) error {
	if *values == nil {
		*values = []ConfigField{}
	}
	seen := map[string]bool{}
	for _, value := range *values {
		if !identifierPattern.MatchString(value.Key) {
			return contractError(ErrorInvalid, "required config key %q is invalid", value.Key)
		}
		if seen[value.Key] {
			return contractError(ErrorDuplicate, "duplicate required config %q", value.Key)
		}
		seen[value.Key] = true
		if value.State != ConfigStatePresent && value.State != ConfigStateMissing {
			return contractError(ErrorInvalid, "required config state %q is invalid", value.State)
		}
		if value.Version != "" && strings.TrimSpace(value.Version) == "" {
			return contractError(ErrorInvalid, "required config %q version is invalid", value.Key)
		}
		if !value.Secret && (value.Version != "" || value.RotatedAt != nil) {
			return contractError(ErrorInvalid, "required config %q has credential metadata but is not secret", value.Key)
		}
		if value.State == ConfigStateMissing && (value.Version != "" || value.RotatedAt != nil) {
			return contractError(ErrorInvalid, "missing required config %q cannot have credential metadata", value.Key)
		}
		if value.RotatedAt != nil && value.RotatedAt.IsZero() {
			return contractError(ErrorInvalid, "required config %q rotatedAt is invalid", value.Key)
		}
	}
	sort.Slice(*values, func(i, j int) bool { return (*values)[i].Key < (*values)[j].Key })
	return nil
}

func normalizeLinks(values *[]Link) error {
	if *values == nil {
		*values = []Link{}
	}
	seen := map[string]bool{}
	for _, value := range *values {
		if strings.TrimSpace(value.Rel) == "" || strings.TrimSpace(value.Href) == "" {
			return contractError(ErrorRequired, "link rel and href are required")
		}
		if seen[value.Rel] {
			return contractError(ErrorDuplicate, "duplicate link rel %q", value.Rel)
		}
		parsed, err := url.Parse(value.Href)
		if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") {
			return contractError(ErrorInvalid, "link href %q must be a queryless same-service absolute path", value.Href)
		}
		seen[value.Rel] = true
	}
	sort.Slice(*values, func(i, j int) bool { return (*values)[i].Rel < (*values)[j].Rel })
	return nil
}

func configurationFrom(values []ConfigField) Configuration {
	present := 0
	for _, value := range values {
		if value.State == ConfigStatePresent {
			present++
		}
	}
	if present == 0 {
		return ConfigurationMissing
	}
	if present == len(values) {
		return ConfigurationComplete
	}
	return ConfigurationPartial
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func effective(support Support, configuration Configuration, enablement Enablement, health Health) bool {
	return support == SupportSupported && configuration == ConfigurationComplete && enablement == EnablementEnabled && health == HealthHealthy
}

func validSupport(value Support) bool {
	return value == SupportSupported || value == SupportUnsupported
}

func validConfiguration(value Configuration) bool {
	return value == ConfigurationMissing || value == ConfigurationPartial || value == ConfigurationComplete
}

func validEnablement(value Enablement) bool {
	return value == EnablementEnabled || value == EnablementDisabled
}

func validHealth(value Health) bool {
	return value == HealthUnknown || value == HealthHealthy || value == HealthDegraded || value == HealthUnhealthy
}

func cloneManifest(value Manifest) Manifest {
	copy := value
	copy.Capabilities = make([]Capability, len(value.Capabilities))
	for index, item := range value.Capabilities {
		copy.Capabilities[index] = cloneCapability(item)
	}
	copy.Providers = make([]Provider, len(value.Providers))
	for index, item := range value.Providers {
		copy.Providers[index] = cloneProvider(item)
	}
	copy.Links = cloneSlice(value.Links)
	return copy
}

func cloneCapability(value Capability) Capability {
	copy := value
	copy.Operations = cloneSlice(value.Operations)
	copy.RequiredConfig = cloneConfig(value.RequiredConfig)
	copy.Links = cloneSlice(value.Links)
	if value.LastCheckedAt != nil {
		lastChecked := *value.LastCheckedAt
		copy.LastCheckedAt = &lastChecked
	}
	return copy
}

func cloneProvider(value Provider) Provider {
	copy := value
	copy.CapabilityKeys = cloneSlice(value.CapabilityKeys)
	copy.Operations = cloneSlice(value.Operations)
	copy.RequiredConfig = cloneConfig(value.RequiredConfig)
	copy.Links = cloneSlice(value.Links)
	if value.LastCheckedAt != nil {
		lastChecked := *value.LastCheckedAt
		copy.LastCheckedAt = &lastChecked
	}
	return copy
}

func cloneConfig(values []ConfigField) []ConfigField {
	copy := cloneSlice(values)
	for index := range copy {
		if copy[index].RotatedAt != nil {
			rotatedAt := *copy[index].RotatedAt
			copy[index].RotatedAt = &rotatedAt
		}
	}
	return copy
}

func cloneSlice[T any](values []T) []T {
	result := make([]T, len(values))
	copy(result, values)
	return result
}
