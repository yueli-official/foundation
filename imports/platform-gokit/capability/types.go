// Package capability defines the versioned, provider-neutral runtime
// capability contract shared by platform foundation services.
package capability

import "time"

const (
	APIVersion = "platform.yueli.dev/service-capability-manifest/v1"
	Kind       = "ServiceCapabilityManifest"
)

type Support string

const (
	SupportSupported   Support = "supported"
	SupportUnsupported Support = "unsupported"
)

type Configuration string

const (
	ConfigurationMissing  Configuration = "missing"
	ConfigurationPartial  Configuration = "partial"
	ConfigurationComplete Configuration = "complete"
)

type Enablement string

const (
	EnablementEnabled  Enablement = "enabled"
	EnablementDisabled Enablement = "disabled"
)

type Health string

const (
	HealthUnknown   Health = "unknown"
	HealthHealthy   Health = "healthy"
	HealthDegraded  Health = "degraded"
	HealthUnhealthy Health = "unhealthy"
)

type ConfigState string

const (
	ConfigStatePresent ConfigState = "present"
	ConfigStateMissing ConfigState = "missing"
)

type ServiceMetadata struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	BuildSHA   string `json:"buildSha"`
	Deployment string `json:"deployment"`
}

// ConfigField exposes presence metadata only. It deliberately has no value
// field, so callers cannot accidentally serialize a credential.
type ConfigField struct {
	Key    string      `json:"key"`
	State  ConfigState `json:"state"`
	Secret bool        `json:"secret"`
}

type Link struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

type Capability struct {
	Key              string        `json:"key"`
	ContractVersion  string        `json:"contractVersion"`
	Support          Support       `json:"support"`
	Configuration    Configuration `json:"configuration"`
	Enablement       Enablement    `json:"enablement"`
	Health           Health        `json:"health"`
	Effective        bool          `json:"effective"`
	Adapter          string        `json:"adapter,omitempty"`
	ProviderInstance string        `json:"providerInstance,omitempty"`
	Operations       []string      `json:"operations"`
	RequiredConfig   []ConfigField `json:"requiredConfig"`
	LastCheckedAt    *time.Time    `json:"lastCheckedAt,omitempty"`
	Links            []Link        `json:"links"`
}

type Provider struct {
	Key            string        `json:"key"`
	Adapter        string        `json:"adapter"`
	CapabilityKeys []string      `json:"capabilityKeys"`
	Configuration  Configuration `json:"configuration"`
	Enablement     Enablement    `json:"enablement"`
	Health         Health        `json:"health"`
	Effective      bool          `json:"effective"`
	Mode           string        `json:"mode,omitempty"`
	Operations     []string      `json:"operations"`
	RequiredConfig []ConfigField `json:"requiredConfig"`
	LastCheckedAt  *time.Time    `json:"lastCheckedAt,omitempty"`
	Links          []Link        `json:"links"`
}

type Manifest struct {
	APIVersion   string          `json:"apiVersion"`
	Kind         string          `json:"kind"`
	Service      ServiceMetadata `json:"service"`
	GeneratedAt  time.Time       `json:"generatedAt"`
	Capabilities []Capability    `json:"capabilities"`
	Providers    []Provider      `json:"providers"`
	Links        []Link          `json:"links"`
}
