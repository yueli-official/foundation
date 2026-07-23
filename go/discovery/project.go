package discovery

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode/utf8"
)

func (module *Module) Project(page PageDescriptor) (PageProjection, Report, error) {
	if module == nil {
		return PageProjection{}, Report{}, failure(ErrorConfiguration, "module_required", "", "module is required")
	}
	if strings.TrimSpace(page.Key) == "" {
		return PageProjection{}, Report{}, failure(ErrorContract, "page_key_required", "key", "is required")
	}
	visibility := page.Visibility
	if visibility == "" {
		visibility = Discoverable
	}
	if visibility == Private {
		return PageProjection{}, Report{}, failure(ErrorConflict, "private_page_not_discoverable", "visibility", "private or authenticated content must not enter Discovery")
	}
	if visibility != Discoverable && visibility != Unlisted {
		return PageProjection{}, Report{}, failure(ErrorContract, "unknown_visibility", "visibility", "is unknown")
	}
	follow := page.Follow
	if follow == "" {
		follow = Follow
	}
	if follow != Follow && follow != NoFollow {
		return PageProjection{}, Report{}, failure(ErrorContract, "unknown_follow_policy", "follow", "is unknown")
	}
	canonical, err := module.absoluteURL(page.Path, "path")
	if err != nil {
		return PageProjection{}, Report{}, err
	}
	title, description, image, modifiedAt, ogType, schemaType, schemaFacts, err := module.subjectFacts(page.Subject)
	if err != nil {
		return PageProjection{}, Report{}, err
	}
	if len(title) > module.limits.MaxTitleBytes {
		return PageProjection{}, Report{}, failure(ErrorCapacity, "title_too_large", "subject.title", "exceeds %d bytes", module.limits.MaxTitleBytes)
	}
	if len(description) > module.limits.MaxDescriptionBytes {
		return PageProjection{}, Report{}, failure(ErrorCapacity, "description_too_large", "subject.description", "exceeds %d bytes", module.limits.MaxDescriptionBytes)
	}
	if page.ModifiedAt != nil {
		value := page.ModifiedAt.UTC()
		modifiedAt = &value
		schemaFacts.modifiedAt = &value
	}
	locale := strings.TrimSpace(page.Locale)
	if locale == "" {
		locale = module.site.DefaultLocale
	}
	links := []LinkTag{{Rel: "canonical", Href: canonical}}
	alternateURLs := make([]AlternateURL, 0, len(page.Alternates))
	seenLocales := map[string]struct{}{}
	for index, alternate := range page.Alternates {
		alternate.Locale = strings.TrimSpace(alternate.Locale)
		if alternate.Locale == "" {
			return PageProjection{}, Report{}, failure(ErrorContract, "alternate_locale_required", fmt.Sprintf("alternates.%d.locale", index), "is required")
		}
		if _, exists := seenLocales[alternate.Locale]; exists {
			return PageProjection{}, Report{}, failure(ErrorConflict, "duplicate_alternate_locale", fmt.Sprintf("alternates.%d.locale", index), "locale %q is duplicated", alternate.Locale)
		}
		seenLocales[alternate.Locale] = struct{}{}
		location, err := module.absoluteURL(alternate.Path, fmt.Sprintf("alternates.%d.path", index))
		if err != nil {
			return PageProjection{}, Report{}, err
		}
		links = append(links, LinkTag{Rel: "alternate", Href: location, HrefLang: alternate.Locale})
		alternateURLs = append(alternateURLs, AlternateURL{Locale: alternate.Locale, Location: location})
	}
	slices.SortFunc(alternateURLs, func(left, right AlternateURL) int {
		return strings.Compare(left.Locale, right.Locale)
	})
	robots := "index," + string(follow)
	if visibility == Unlisted {
		robots = "noindex," + string(follow)
	}
	meta := []MetaTag{
		{Name: "description", Content: description},
		{Name: "robots", Content: robots},
		{Property: "og:site_name", Content: module.site.Name},
		{Property: "og:title", Content: title},
		{Property: "og:description", Content: description},
		{Property: "og:type", Content: ogType},
		{Property: "og:url", Content: canonical},
		{Property: "og:locale", Content: locale},
	}
	report := Report{Diagnostics: []Diagnostic{}}
	if image == nil {
		image = module.site.DefaultImage
	}
	if image != nil {
		imageURL, err := module.absoluteURL(image.URL, "subject.image.url")
		if err != nil {
			return PageProjection{}, Report{}, err
		}
		meta = append(meta, MetaTag{Property: "og:image", Content: imageURL})
		if image.Alt != "" {
			meta = append(meta, MetaTag{Property: "og:image:alt", Content: image.Alt})
		} else {
			report.Diagnostics = append(report.Diagnostics, Diagnostic{
				Code: "image_alt_recommended", Severity: SeverityWarning,
				Path: "subject.image.alt", Protocol: "open-graph",
				Reference: "https://ogp.me/", Message: "an image alternate description is recommended",
			})
		}
		if image.Width > 0 {
			meta = append(meta, MetaTag{Property: "og:image:width", Content: fmt.Sprint(image.Width)})
		}
		if image.Height > 0 {
			meta = append(meta, MetaTag{Property: "og:image:height", Content: fmt.Sprint(image.Height)})
		}
		if image.Type != "" {
			meta = append(meta, MetaTag{Property: "og:image:type", Content: image.Type})
		}
	}
	card := "summary"
	if image != nil {
		card = "summary_large_image"
	}
	meta = append(meta,
		MetaTag{Name: "twitter:card", Content: card},
		MetaTag{Name: "twitter:title", Content: title},
		MetaTag{Name: "twitter:description", Content: description},
	)
	if image != nil {
		imageURL, _ := module.absoluteURL(image.URL, "subject.image.url")
		meta = append(meta, MetaTag{Name: "twitter:image", Content: imageURL})
		if image.Alt != "" {
			meta = append(meta, MetaTag{Name: "twitter:image:alt", Content: image.Alt})
		}
	}
	graph, err := module.structuredData(page, canonical, locale, schemaType, schemaFacts)
	if err != nil {
		return PageProjection{}, Report{}, err
	}
	jsonLD, err := json.Marshal(graph)
	if err != nil {
		return PageProjection{}, Report{}, &Error{
			Kind: ErrorEncoding, Code: "structured_data_encoding_failed",
			Path: "subject", Protocol: "json-ld", Cause: err,
		}
	}
	if len(jsonLD) > module.limits.MaxPageJSONBytes {
		return PageProjection{}, Report{}, failure(ErrorCapacity, "structured_data_too_large", "subject", "JSON-LD exceeds %d bytes", module.limits.MaxPageJSONBytes)
	}
	projection := PageProjection{
		ContractVersion: ContractVersion,
		Key:             page.Key, CanonicalURL: canonical,
		Head: HeadProjection{
			Title: title, Links: links, Meta: meta,
			StructuredData: []StructuredData{{ID: "discovery-jsonld", JSON: jsonLD}},
		},
		Headers: HeaderProjection{
			Link:       []string{"<" + canonical + `>; rel="canonical"`},
			XRobotsTag: robots,
		},
		Diagnostics: report.Diagnostics,
	}
	if visibility == Discoverable {
		projection.Sitemap = &SitemapEntry{
			Location: canonical, LastModified: modifiedAt,
			Alternates: alternateURLs,
		}
	}
	encodedProjection, err := json.Marshal(projection)
	if err != nil {
		return PageProjection{}, Report{}, &Error{
			Kind: ErrorEncoding, Code: "page_projection_encoding_failed",
			Path: "page", Cause: err,
		}
	}
	if len(encodedProjection) > module.limits.MaxPageJSONBytes {
		return PageProjection{}, Report{}, failure(ErrorCapacity, "page_projection_too_large", "page", "projection exceeds %d bytes", module.limits.MaxPageJSONBytes)
	}
	return projection, report, nil
}

func (module *Module) absoluteURL(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", failure(ErrorContract, "url_required", field, "is required")
	}
	reference, err := url.Parse(value)
	if err != nil {
		return "", failure(ErrorContract, "invalid_url", field, "is not a valid URL")
	}
	if reference.Fragment != "" || reference.User != nil {
		return "", failure(ErrorContract, "invalid_url", field, "must not contain credentials or a fragment")
	}
	if !module.urlPolicy.PreserveQuery && reference.RawQuery != "" {
		return "", failure(ErrorConflict, "query_not_allowed", field, "query requires an explicit preserving URL policy")
	}
	if reference.IsAbs() {
		if !sameOrigin(module.origin, reference) {
			return "", failure(ErrorConflict, "canonical_origin_mismatch", field, "must use configured site origin")
		}
	} else {
		if !strings.HasPrefix(reference.Path, "/") || strings.HasPrefix(reference.Path, "//") {
			return "", failure(ErrorContract, "absolute_path_required", field, "must be an absolute-path reference")
		}
		reference = module.origin.ResolveReference(reference)
	}
	cleaned := path.Clean(reference.EscapedPath())
	if cleaned == "." {
		cleaned = "/"
	}
	decoded, err := url.PathUnescape(cleaned)
	if err != nil || !utf8.ValidString(decoded) {
		return "", failure(ErrorContract, "invalid_url_encoding", field, "contains invalid path encoding")
	}
	if module.urlPolicy.TrailingSlash && cleaned != "/" {
		cleaned = strings.TrimSuffix(cleaned, "/") + "/"
	} else if !module.urlPolicy.TrailingSlash && cleaned != "/" {
		cleaned = strings.TrimSuffix(cleaned, "/")
	}
	reference.Scheme = strings.ToLower(reference.Scheme)
	reference.Host = strings.ToLower(reference.Host)
	reference.Path = decoded
	reference.RawPath = ""
	return reference.String(), nil
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Host, right.Host)
}
