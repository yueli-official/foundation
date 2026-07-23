package urllifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const DefinitionVersion uint64 = 1

var definitionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

type QueryKeyDefinition struct {
	Key         string `json:"key"`
	Default     string `json:"default,omitempty"`
	OmitDefault bool   `json:"omitDefault,omitempty"`
}

type QueryIdentityDefinition struct {
	Keys     []QueryKeyDefinition `json:"keys,omitempty"`
	MaxBytes int                  `json:"maxBytes,omitempty"`
}

type NamespaceDefinition struct {
	Key           Namespace               `json:"key"`
	PathPrefix    string                  `json:"pathPrefix"`
	IdentityQuery QueryIdentityDefinition `json:"identityQuery,omitempty"`
}

type ResourceKindDefinition struct {
	Key ResourceKind `json:"key"`
}

type Limits struct {
	MaxPathBytes       int `json:"maxPathBytes,omitempty"`
	MaxQueryBytes      int `json:"maxQueryBytes,omitempty"`
	MaxResourceIDBytes int `json:"maxResourceIdBytes,omitempty"`
	MaxVariantBytes    int `json:"maxVariantBytes,omitempty"`
	MaxReasonBytes     int `json:"maxReasonBytes,omitempty"`
	MaxChanges         int `json:"maxChanges,omitempty"`
	MaxAliasesPerRoute int `json:"maxAliasesPerRoute,omitempty"`
	MaxDiagnostics     int `json:"maxDiagnostics,omitempty"`
	MaxPageSize        int `json:"maxPageSize,omitempty"`
}

type Definition struct {
	Version         uint64                   `json:"version"`
	TrustedOrigin   string                   `json:"trustedOrigin"`
	ResourceKinds   []ResourceKindDefinition `json:"resourceKinds"`
	Namespaces      []NamespaceDefinition    `json:"namespaces"`
	ExternalOrigins []string                 `json:"externalOrigins,omitempty"`
	Limits          Limits                   `json:"limits,omitempty"`
}

type compiledQueryKey struct {
	key         string
	defaultVal  string
	omitDefault bool
}

type compiledNamespace struct {
	key        Namespace
	pathPrefix string
	queryKeys  []compiledQueryKey
	querySet   map[string]int
	maxBytes   int
}

type Catalog struct {
	version         uint64
	trustedOrigin   string
	trusted         normalizedOrigin
	resourceKinds   map[ResourceKind]struct{}
	namespaces      []compiledNamespace
	externalOrigins map[normalizedOrigin]struct{}
	limits          Limits
	digest          Digest
}

type normalizedOrigin struct {
	scheme string
	host   string
	port   string
}

func Compile(definition Definition) (*Catalog, error) {
	if definition.Version != DefinitionVersion {
		return nil, invalid("version", "must equal %d", DefinitionVersion)
	}
	trusted, canonicalTrusted, err := normalizeOrigin(definition.TrustedOrigin)
	if err != nil {
		return nil, invalid("trusted_origin", "%v", err)
	}
	kinds := make(map[ResourceKind]struct{}, len(definition.ResourceKinds))
	canonicalKinds := make([]string, 0, len(definition.ResourceKinds))
	if len(definition.ResourceKinds) == 0 {
		return nil, invalid("resource_kinds", "must contain at least one kind")
	}
	for index, item := range definition.ResourceKinds {
		key := ResourceKind(strings.TrimSpace(string(item.Key)))
		if !definitionKeyPattern.MatchString(string(key)) {
			return nil, invalid("resource_kinds", "item %d has an invalid key", index)
		}
		if _, exists := kinds[key]; exists {
			return nil, invalid("resource_kinds", "contains duplicate %q", key)
		}
		kinds[key] = struct{}{}
		canonicalKinds = append(canonicalKinds, string(key))
	}
	slices.Sort(canonicalKinds)

	if len(definition.Namespaces) == 0 {
		return nil, invalid("namespaces", "must contain at least one namespace")
	}
	namespaces := make([]compiledNamespace, 0, len(definition.Namespaces))
	canonicalNamespaces := make([]NamespaceDefinition, 0, len(definition.Namespaces))
	seenNamespaces := map[Namespace]struct{}{}
	seenPrefixes := map[string]struct{}{}
	for index, item := range definition.Namespaces {
		key := Namespace(strings.TrimSpace(string(item.Key)))
		if !definitionKeyPattern.MatchString(string(key)) {
			return nil, invalid("namespaces", "item %d has an invalid key", index)
		}
		if _, exists := seenNamespaces[key]; exists {
			return nil, invalid("namespaces", "contains duplicate key %q", key)
		}
		prefix, err := normalizePath(item.PathPrefix, 4096)
		if err != nil {
			return nil, invalid("namespaces", "item %d path prefix: %v", index, err)
		}
		if prefix != "/" {
			prefix = strings.TrimSuffix(prefix, "/")
		}
		if _, exists := seenPrefixes[prefix]; exists {
			return nil, invalid("namespaces", "contains duplicate path prefix %q", prefix)
		}
		if item.IdentityQuery.MaxBytes < 0 {
			return nil, invalid("namespaces", "item %d query max bytes cannot be negative", index)
		}
		maxBytes := item.IdentityQuery.MaxBytes
		if maxBytes == 0 {
			maxBytes = 4096
		}
		queryKeys := make([]compiledQueryKey, 0, len(item.IdentityQuery.Keys))
		querySet := make(map[string]int, len(item.IdentityQuery.Keys))
		canonicalQueryKeys := make([]QueryKeyDefinition, 0, len(item.IdentityQuery.Keys))
		for queryIndex, query := range item.IdentityQuery.Keys {
			queryKey := strings.TrimSpace(query.Key)
			if !definitionKeyPattern.MatchString(queryKey) {
				return nil, invalid("namespaces", "item %d query key %d is invalid", index, queryIndex)
			}
			if _, exists := querySet[queryKey]; exists {
				return nil, invalid("namespaces", "item %d contains duplicate query key %q", index, queryKey)
			}
			querySet[queryKey] = len(queryKeys)
			queryKeys = append(queryKeys, compiledQueryKey{
				key: queryKey, defaultVal: query.Default, omitDefault: query.OmitDefault,
			})
			canonicalQueryKeys = append(canonicalQueryKeys, QueryKeyDefinition{
				Key: queryKey, Default: query.Default, OmitDefault: query.OmitDefault,
			})
		}
		seenNamespaces[key] = struct{}{}
		seenPrefixes[prefix] = struct{}{}
		namespaces = append(namespaces, compiledNamespace{
			key: key, pathPrefix: prefix, queryKeys: queryKeys, querySet: querySet, maxBytes: maxBytes,
		})
		canonicalNamespaces = append(canonicalNamespaces, NamespaceDefinition{
			Key: key, PathPrefix: prefix,
			IdentityQuery: QueryIdentityDefinition{Keys: canonicalQueryKeys, MaxBytes: maxBytes},
		})
	}
	slices.SortFunc(namespaces, func(left, right compiledNamespace) int {
		if len(left.pathPrefix) != len(right.pathPrefix) {
			return len(right.pathPrefix) - len(left.pathPrefix)
		}
		return strings.Compare(string(left.key), string(right.key))
	})
	slices.SortFunc(canonicalNamespaces, func(left, right NamespaceDefinition) int {
		return strings.Compare(string(left.Key), string(right.Key))
	})

	external := make(map[normalizedOrigin]struct{}, len(definition.ExternalOrigins))
	canonicalExternal := make([]string, 0, len(definition.ExternalOrigins))
	for index, raw := range definition.ExternalOrigins {
		origin, canonical, err := normalizeOrigin(raw)
		if err != nil {
			return nil, invalid("external_origins", "item %d: %v", index, err)
		}
		if _, exists := external[origin]; exists {
			return nil, invalid("external_origins", "contains duplicate %q", canonical)
		}
		external[origin] = struct{}{}
		canonicalExternal = append(canonicalExternal, canonical)
	}
	slices.Sort(canonicalExternal)

	limits, err := normalizeLimits(definition.Limits)
	if err != nil {
		return nil, err
	}
	canonical := Definition{
		Version: definition.Version, TrustedOrigin: canonicalTrusted, Limits: limits,
		Namespaces: canonicalNamespaces,
	}
	for _, key := range canonicalKinds {
		canonical.ResourceKinds = append(canonical.ResourceKinds, ResourceKindDefinition{Key: ResourceKind(key)})
	}
	canonical.ExternalOrigins = canonicalExternal
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, &Error{Kind: ErrorUnavailable, Field: "definition", Message: "cannot encode canonical definition", Cause: err}
	}
	sum := sha256.Sum256(encoded)
	return &Catalog{
		version: definition.Version, trustedOrigin: canonicalTrusted, trusted: trusted,
		resourceKinds: kinds, namespaces: namespaces, externalOrigins: external,
		limits: limits, digest: Digest(hex.EncodeToString(sum[:])),
	}, nil
}

func MustCompile(definition Definition) *Catalog {
	catalog, err := Compile(definition)
	if err != nil {
		panic(err)
	}
	return catalog
}

func (catalog *Catalog) Version() uint64 {
	if catalog == nil {
		return 0
	}
	return catalog.version
}

func (catalog *Catalog) Digest() Digest {
	if catalog == nil {
		return ""
	}
	return catalog.digest
}

func (catalog *Catalog) TrustedOrigin() string {
	if catalog == nil {
		return ""
	}
	return catalog.trustedOrigin
}

func normalizeLimits(value Limits) (Limits, error) {
	if value.MaxPathBytes == 0 {
		value.MaxPathBytes = 4096
	}
	if value.MaxQueryBytes == 0 {
		value.MaxQueryBytes = 4096
	}
	if value.MaxResourceIDBytes == 0 {
		value.MaxResourceIDBytes = 200
	}
	if value.MaxVariantBytes == 0 {
		value.MaxVariantBytes = 200
	}
	if value.MaxReasonBytes == 0 {
		value.MaxReasonBytes = 2000
	}
	if value.MaxChanges == 0 {
		value.MaxChanges = 50_000
	}
	if value.MaxAliasesPerRoute == 0 {
		value.MaxAliasesPerRoute = 100
	}
	if value.MaxDiagnostics == 0 {
		value.MaxDiagnostics = 1000
	}
	if value.MaxPageSize == 0 {
		value.MaxPageSize = 200
	}
	values := []struct {
		name  string
		value int
	}{
		{"max_path_bytes", value.MaxPathBytes},
		{"max_query_bytes", value.MaxQueryBytes},
		{"max_resource_id_bytes", value.MaxResourceIDBytes},
		{"max_variant_bytes", value.MaxVariantBytes},
		{"max_reason_bytes", value.MaxReasonBytes},
		{"max_changes", value.MaxChanges},
		{"max_aliases_per_route", value.MaxAliasesPerRoute},
		{"max_diagnostics", value.MaxDiagnostics},
		{"max_page_size", value.MaxPageSize},
	}
	for _, item := range values {
		if item.value <= 0 {
			return Limits{}, invalid("limits."+item.name, "must be positive")
		}
	}
	return value, nil
}

func normalizeOrigin(raw string) (normalizedOrigin, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return normalizedOrigin{}, "", err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return normalizedOrigin{}, "", invalid("origin", "must use http or https")
	}
	if parsed.User != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return normalizedOrigin{}, "", invalid("origin", "must be an origin without userinfo, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return normalizedOrigin{}, "", invalid("origin", "must not contain a path")
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return normalizedOrigin{}, "", invalid("origin", "contains an invalid port")
		}
	}
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	hostPort := host
	if strings.Contains(host, ":") {
		hostPort = "[" + host + "]"
	}
	if port != "" {
		hostPort = net.JoinHostPort(host, port)
	}
	origin := normalizedOrigin{scheme: parsed.Scheme, host: host, port: port}
	return origin, parsed.Scheme + "://" + hostPort, nil
}
