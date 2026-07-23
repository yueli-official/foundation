package discovery

import (
	"net/url"
	"strings"
)

const (
	defaultMaxTitleBytes          = 512
	defaultMaxDescriptionBytes    = 4096
	defaultMaxPageJSONBytes       = 256 * 1024
	defaultMaxStructuredDataNodes = 128
	defaultMaxSourceBatch         = 500
	defaultMaxFeedEntries         = 500
	defaultMaxSitemapURLs         = 50_000
	defaultMaxSitemapBytes        = 52_428_800
	defaultMaxSitemapParts        = 50_000
	defaultMaxRobotsBytes         = 500 * 1024
)

type Module struct {
	origin    *url.URL
	site      SiteProfile
	urlPolicy URLPolicy
	limits    Limits
}

func Compile(definition Definition) (*Module, error) {
	if definition.ContractVersion == "" {
		definition.ContractVersion = ContractVersion
	}
	if definition.ContractVersion != ContractVersion {
		return nil, failure(ErrorUnsupported, "unsupported_contract_version", "contractVersion", "must equal %q", ContractVersion)
	}
	origin, err := url.Parse(strings.TrimSpace(definition.Site.Origin))
	if err != nil || origin.Scheme == "" || origin.Host == "" {
		return nil, failure(ErrorConfiguration, "invalid_site_origin", "site.origin", "must be an absolute HTTP(S) origin")
	}
	if origin.Scheme != "http" && origin.Scheme != "https" {
		return nil, failure(ErrorConfiguration, "invalid_site_origin", "site.origin", "scheme must be HTTP or HTTPS")
	}
	if origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return nil, failure(ErrorConfiguration, "invalid_site_origin", "site.origin", "must not contain credentials, path, query, or fragment")
	}
	origin.Path = ""
	if strings.TrimSpace(definition.Site.Name) == "" {
		return nil, failure(ErrorConfiguration, "site_name_required", "site.name", "is required")
	}
	if strings.TrimSpace(definition.Site.DefaultLocale) == "" {
		return nil, failure(ErrorConfiguration, "default_locale_required", "site.defaultLocale", "is required")
	}
	limits, err := normalizeLimits(definition.Limits)
	if err != nil {
		return nil, err
	}
	module := &Module{
		origin: origin, site: definition.Site,
		urlPolicy: definition.URLPolicy, limits: limits,
	}
	if definition.Site.DefaultImage != nil {
		if _, err := module.absoluteURL(definition.Site.DefaultImage.URL, "site.defaultImage.url"); err != nil {
			return nil, err
		}
	}
	return module, nil
}

func MustCompile(definition Definition) *Module {
	module, err := Compile(definition)
	if err != nil {
		panic(err)
	}
	return module
}

func normalizeLimits(value Limits) (Limits, error) {
	defaultInt(&value.MaxTitleBytes, defaultMaxTitleBytes)
	defaultInt(&value.MaxDescriptionBytes, defaultMaxDescriptionBytes)
	defaultInt(&value.MaxPageJSONBytes, defaultMaxPageJSONBytes)
	defaultInt(&value.MaxStructuredDataNodes, defaultMaxStructuredDataNodes)
	defaultInt(&value.MaxSourceBatch, defaultMaxSourceBatch)
	defaultInt(&value.MaxFeedEntries, defaultMaxFeedEntries)
	defaultInt(&value.MaxSitemapURLs, defaultMaxSitemapURLs)
	defaultInt(&value.MaxSitemapBytes, defaultMaxSitemapBytes)
	defaultInt(&value.MaxSitemapParts, defaultMaxSitemapParts)
	defaultInt(&value.MaxRobotsBytes, defaultMaxRobotsBytes)
	checks := []struct {
		path       string
		value, max int
	}{
		{"limits.maxTitleBytes", value.MaxTitleBytes, 16 * 1024},
		{"limits.maxDescriptionBytes", value.MaxDescriptionBytes, 64 * 1024},
		{"limits.maxPageJsonBytes", value.MaxPageJSONBytes, 4 * 1024 * 1024},
		{"limits.maxStructuredDataNodes", value.MaxStructuredDataNodes, 4096},
		{"limits.maxSourceBatch", value.MaxSourceBatch, 10_000},
		{"limits.maxFeedEntries", value.MaxFeedEntries, 10_000},
		{"limits.maxSitemapUrls", value.MaxSitemapURLs, defaultMaxSitemapURLs},
		{"limits.maxSitemapBytes", value.MaxSitemapBytes, defaultMaxSitemapBytes},
		{"limits.maxSitemapParts", value.MaxSitemapParts, defaultMaxSitemapParts},
		{"limits.maxRobotsBytes", value.MaxRobotsBytes, defaultMaxRobotsBytes},
	}
	for _, check := range checks {
		if check.value < 1 || check.value > check.max {
			return Limits{}, failure(ErrorConfiguration, "invalid_limit", check.path, "must be between 1 and %d", check.max)
		}
	}
	return value, nil
}

func defaultInt(target *int, value int) {
	if *target == 0 {
		*target = value
	}
}
