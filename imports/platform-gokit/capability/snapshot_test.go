package capability

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewSnapshotNormalizesEffectiveAndSorts(t *testing.T) {
	checkedAt := time.Date(2026, 7, 12, 8, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	snapshot, err := NewSnapshot(Manifest{
		Service:     ServiceMetadata{Name: "notification", Version: "1.2.3", BuildSHA: strings.Repeat("a", 40), Deployment: "notification-api"},
		GeneratedAt: checkedAt,
		Redaction:   RedactionMetadata{Policy: "presence-only", Version: "1"},
		Capabilities: []Capability{
			{Key: "notification.sms", ContractVersion: "1.0", Support: SupportUnsupported, Configuration: ConfigurationMissing, Enablement: EnablementDisabled, Health: HealthUnknown, Effective: true},
			{Key: "notification.email", ContractVersion: "1.0", Support: SupportSupported, Configuration: ConfigurationComplete, Enablement: EnablementEnabled, Health: HealthHealthy},
		},
		Providers: []Provider{
			{Key: "secondary", Adapter: "dev", CapabilityKeys: []string{"notification.email"}, Configuration: ConfigurationComplete, Enablement: EnablementDisabled, Health: HealthHealthy, Effective: true},
			{Key: "primary", Adapter: "smtp", CapabilityKeys: []string{"notification.email"}, Configuration: ConfigurationComplete, Enablement: EnablementEnabled, Health: HealthHealthy, LastCheckedAt: &checkedAt},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	manifest := snapshot.Manifest()
	if manifest.APIVersion != APIVersion || manifest.Kind != Kind {
		t.Fatalf("contract identity = %s %s", manifest.APIVersion, manifest.Kind)
	}
	if got := []string{manifest.Capabilities[0].Key, manifest.Capabilities[1].Key}; !reflect.DeepEqual(got, []string{"notification.email", "notification.sms"}) {
		t.Fatalf("capability order = %v", got)
	}
	if !manifest.Capabilities[0].Effective || manifest.Capabilities[1].Effective {
		t.Fatalf("capability effective states = %+v", manifest.Capabilities)
	}
	if got := []string{manifest.Providers[0].Key, manifest.Providers[1].Key}; !reflect.DeepEqual(got, []string{"primary", "secondary"}) {
		t.Fatalf("provider order = %v", got)
	}
	if !manifest.Providers[0].Effective || manifest.Providers[1].Effective {
		t.Fatalf("provider effective states = %+v", manifest.Providers)
	}
}

func TestSnapshotListGetAndCopiesAreConsistent(t *testing.T) {
	snapshot, err := NewSnapshot(validManifest())
	if err != nil {
		t.Fatal(err)
	}

	listed := snapshot.ListCapabilities()
	got, ok := snapshot.Capability("asset.object-storage")
	if !ok || !reflect.DeepEqual(got, listed[0]) {
		t.Fatalf("get capability = %+v, %t; list = %+v", got, ok, listed)
	}
	provider, ok := snapshot.Provider("primary-s3")
	if !ok || !reflect.DeepEqual(provider, snapshot.ListProviders()[0]) {
		t.Fatalf("get provider = %+v, %t", provider, ok)
	}
	if _, ok := snapshot.Capability("missing"); ok {
		t.Fatal("missing capability unexpectedly found")
	}
	if _, ok := snapshot.Provider("missing"); ok {
		t.Fatal("missing provider unexpectedly found")
	}

	listed[0].RequiredConfig[0].State = ConfigStateMissing
	listed[0].Operations[0] = "mutated"
	provider.CapabilityKeys[0] = "mutated"
	provider.RequiredConfig[0].State = ConfigStateMissing
	freshCapability, _ := snapshot.Capability("asset.object-storage")
	freshProvider, _ := snapshot.Provider("primary-s3")
	if freshCapability.RequiredConfig[0].State != ConfigStatePresent || freshCapability.Operations[0] != "presign_get" {
		t.Fatalf("snapshot capability was mutated through returned copy: %+v", freshCapability)
	}
	if freshProvider.CapabilityKeys[0] != "asset.object-storage" || freshProvider.RequiredConfig[0].State != ConfigStatePresent {
		t.Fatalf("snapshot provider was mutated through returned copy: %+v", freshProvider)
	}
}

func TestSnapshotDerivesConfigurationAndProviderBinding(t *testing.T) {
	value := validManifest()
	value.Capabilities[0].Configuration = ConfigurationComplete
	value.Capabilities[0].ProviderInstance = "primary-s3"
	value.Capabilities[0].Adapter = ""
	value.Capabilities[0].RequiredConfig[1].State = ConfigStateMissing
	value.Capabilities[0].RequiredConfig[1].Version = ""
	value.Capabilities[0].RequiredConfig[1].RotatedAt = nil
	value.Providers[0].Configuration = ConfigurationComplete
	value.Providers[0].RequiredConfig[1].State = ConfigStateMissing
	value.Providers[0].RequiredConfig[1].Version = ""
	value.Providers[0].RequiredConfig[1].RotatedAt = nil
	snapshot, err := NewSnapshot(value)
	if err != nil {
		t.Fatal(err)
	}
	capability, _ := snapshot.Capability("asset.object-storage")
	provider, _ := snapshot.Provider("primary-s3")
	if capability.Configuration != ConfigurationPartial || capability.Effective || capability.Adapter != "s3" {
		t.Fatalf("derived capability = %+v", capability)
	}
	if provider.Configuration != ConfigurationPartial || provider.Effective {
		t.Fatalf("derived provider = %+v", provider)
	}
	if capability.RequiredConfig[0].Key != "endpoint" || capability.RequiredConfig[1].Key != "secret_key" {
		t.Fatalf("required config order = %+v", capability.RequiredConfig)
	}
}

func TestNewSnapshotRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"service name", func(value *Manifest) { value.Service.Name = "" }, "service.name"},
		{"generated at", func(value *Manifest) { value.GeneratedAt = time.Time{} }, "generatedAt"},
		{"redaction", func(value *Manifest) { value.Redaction.Version = "" }, "redaction"},
		{"duplicate capability", func(value *Manifest) { value.Capabilities = append(value.Capabilities, value.Capabilities[0]) }, "duplicate capability"},
		{"unknown support", func(value *Manifest) { value.Capabilities[0].Support = "maybe" }, "support"},
		{"unknown configuration", func(value *Manifest) { value.Capabilities[0].Configuration = "maybe" }, "configuration"},
		{"unknown enablement", func(value *Manifest) { value.Capabilities[0].Enablement = "maybe" }, "enablement"},
		{"unknown health", func(value *Manifest) { value.Capabilities[0].Health = "maybe" }, "health"},
		{"duplicate config key", func(value *Manifest) {
			value.Capabilities[0].RequiredConfig = append(value.Capabilities[0].RequiredConfig, value.Capabilities[0].RequiredConfig[0])
		}, "duplicate required config"},
		{"duplicate provider", func(value *Manifest) { value.Providers = append(value.Providers, value.Providers[0]) }, "duplicate provider"},
		{"unknown provider capability", func(value *Manifest) { value.Providers[0].CapabilityKeys = []string{"asset.missing"} }, "unknown capability"},
		{"unsupported provider capability", func(value *Manifest) { value.Capabilities[0].Support = SupportUnsupported }, "unsupported capability"},
		{"empty adapter", func(value *Manifest) { value.Providers[0].Adapter = "" }, "adapter"},
		{"duplicate operation", func(value *Manifest) { value.Providers[0].Operations = []string{"put", "put"} }, "duplicate operation"},
		{"secret state invalid", func(value *Manifest) { value.Providers[0].RequiredConfig[1].State = "secret-value" }, "required config state"},
		{"missing provider instance", func(value *Manifest) { value.Capabilities[0].ProviderInstance = "missing" }, "unknown provider instance"},
		{"provider adapter mismatch", func(value *Manifest) {
			value.Capabilities[0].ProviderInstance = "primary-s3"
			value.Capabilities[0].Adapter = "local"
		}, "does not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validManifest()
			test.mutate(&value)
			if _, err := NewSnapshot(value); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewSnapshot() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestContractErrorsAreMachineClassifiable(t *testing.T) {
	value := validManifest()
	value.Capabilities[0].Health = "maybe"
	_, err := NewSnapshot(value)
	var contractErr *ContractError
	if !errors.As(err, &contractErr) || contractErr.Code != ErrorInvalid {
		t.Fatalf("NewSnapshot() error = %#v, want invalid ContractError", err)
	}
}

func TestSnapshotRejectsUnsafeLinks(t *testing.T) {
	for _, href := range []string{"https://example.com/admin", "//example.com/admin", "/admin?token=secret", "/admin#secret"} {
		t.Run(href, func(t *testing.T) {
			value := validManifest()
			value.Links[0].Href = href
			if _, err := NewSnapshot(value); err == nil || !strings.Contains(err.Error(), "queryless same-service") {
				t.Fatalf("NewSnapshot() error = %v", err)
			}
		})
	}
}

func TestManifestJSONCannotContainSecretValues(t *testing.T) {
	snapshot, err := NewSnapshot(validManifest())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(snapshot.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, forbidden := range []string{"secretValue", "passwordValue", "accessKeyValue"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("manifest JSON leaked forbidden field %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, `"secret":true`) || !strings.Contains(encoded, `"state":"present"`) {
		t.Fatalf("manifest JSON lost redacted secret metadata: %s", encoded)
	}
}

func validManifest() Manifest {
	checkedAt := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	return Manifest{
		Service:     ServiceMetadata{Name: "asset", Version: "1.0.0", BuildSHA: strings.Repeat("b", 40), Deployment: "asset-api"},
		GeneratedAt: checkedAt,
		Redaction:   RedactionMetadata{Policy: "presence-only", Version: "1"},
		Capabilities: []Capability{{
			Key: "asset.object-storage", ContractVersion: "1.0", Support: SupportSupported,
			Configuration: ConfigurationComplete, Enablement: EnablementEnabled, Health: HealthHealthy,
			Operations:     []string{"presign_get", "presign_put"},
			RequiredConfig: []ConfigField{{Key: "endpoint", State: ConfigStatePresent}, {Key: "secret_key", State: ConfigStatePresent, Secret: true, Version: "3", RotatedAt: &checkedAt}},
		}},
		Providers: []Provider{{
			Key: "primary-s3", Adapter: "s3", CapabilityKeys: []string{"asset.object-storage"},
			Configuration: ConfigurationComplete, Enablement: EnablementEnabled, Health: HealthHealthy,
			Operations: []string{"get", "put"}, Mode: "production", LastCheckedAt: &checkedAt,
			RequiredConfig: []ConfigField{{Key: "endpoint", State: ConfigStatePresent}, {Key: "secret_key", State: ConfigStatePresent, Secret: true, Version: "3", RotatedAt: &checkedAt}},
		}},
		Links: []Link{{Rel: "health", Href: "/healthz"}, {Rel: "ready", Href: "/readyz"}},
	}
}
