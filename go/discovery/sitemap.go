package discovery

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

const sitemapHeader = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
	`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n"
const sitemapFooter = "</urlset>\n"

type sitemapPart struct {
	artifact Artifact
	location string
	lastmod  *time.Time
}

func (module *Module) publishSitemap(
	ctx context.Context,
	plan SitemapPlan,
	sources Sources,
	transaction PublicationWriter,
) ([]Artifact, string, Report, error) {
	route, err := normalizeArtifactRoute(plan.Route, "sitemap.xml", "sitemap.route")
	if err != nil {
		return nil, "", Report{}, err
	}
	source, err := module.source(plan.Source, sources, "sitemap.source")
	if err != nil {
		return nil, "", Report{}, err
	}
	extension := path.Ext(route)
	base := strings.TrimSuffix(route, extension)
	if extension == "" {
		extension = ".xml"
	}
	var parts []sitemapPart
	var current *measuredWriter
	var currentName string
	currentEntries := 0
	var currentLastmod *time.Time
	closePart := func() error {
		if current == nil {
			return nil
		}
		if err := writeString(current, sitemapFooter); err != nil {
			return err
		}
		if err := current.Close(); err != nil {
			return targetError("target_close_failed", true, err)
		}
		location, err := module.absoluteURL("/"+currentName, "sitemap.part")
		if err != nil {
			return err
		}
		parts = append(parts, sitemapPart{
			artifact: current.artifact(currentName, "application/xml; charset=utf-8"),
			location: location, lastmod: currentLastmod,
		})
		current = nil
		currentEntries = 0
		currentLastmod = nil
		return nil
	}
	openPart := func() error {
		if len(parts) >= module.limits.MaxSitemapParts {
			return failure(ErrorCapacity, "sitemap_part_limit", "sitemap", "exceeds %d parts", module.limits.MaxSitemapParts)
		}
		currentName = fmt.Sprintf("%s-%05d%s", base, len(parts)+1, extension)
		writer, err := createArtifact(ctx, transaction, currentName, "application/xml; charset=utf-8")
		if err != nil {
			return err
		}
		current = writer
		return writeString(current, sitemapHeader)
	}
	if err := openPart(); err != nil {
		return nil, "", Report{}, err
	}
	report := Report{Diagnostics: []Diagnostic{}}
	err = module.scan(ctx, source, 0, func(record Record) error {
		projection, pageReport, err := module.Project(record.Page)
		report.Diagnostics = append(report.Diagnostics, pageReport.Diagnostics...)
		if err != nil {
			return err
		}
		if projection.Sitemap == nil {
			return nil
		}
		if record.SortKey != projection.CanonicalURL {
			return failure(ErrorConflict, "source_sort_key_mismatch", "record.sortKey", "must equal the canonical URL for sitemap sources")
		}
		encoded := encodeSitemapEntry(*projection.Sitemap)
		entryBytes := len(encoded)
		if len(sitemapHeader)+entryBytes+len(sitemapFooter) > module.limits.MaxSitemapBytes {
			return failure(ErrorCapacity, "sitemap_entry_too_large", "record.page", "single entry exceeds sitemap byte limit")
		}
		if currentEntries >= module.limits.MaxSitemapURLs ||
			int(current.bytes)+entryBytes+len(sitemapFooter) > module.limits.MaxSitemapBytes {
			if err := closePart(); err != nil {
				return err
			}
			if err := openPart(); err != nil {
				return err
			}
		}
		if err := writeString(current, encoded); err != nil {
			return err
		}
		currentEntries++
		if projection.Sitemap.LastModified != nil &&
			(currentLastmod == nil || projection.Sitemap.LastModified.After(*currentLastmod)) {
			value := projection.Sitemap.LastModified.UTC()
			currentLastmod = &value
		}
		return nil
	})
	if err != nil {
		return nil, "", report, err
	}
	if err := closePart(); err != nil {
		return nil, "", report, err
	}
	indexWriter, err := createArtifact(ctx, transaction, route, "application/xml; charset=utf-8")
	if err != nil {
		return nil, "", report, err
	}
	if err := writeString(indexWriter, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
		`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`+"\n"); err != nil {
		return nil, "", report, err
	}
	for _, part := range parts {
		entry := "  <sitemap><loc>" + xmlText(part.location) + "</loc>"
		if part.lastmod != nil {
			entry += "<lastmod>" + part.lastmod.UTC().Format(time.RFC3339) + "</lastmod>"
		}
		entry += "</sitemap>\n"
		if int(indexWriter.bytes)+len(entry)+len("</sitemapindex>\n") > module.limits.MaxSitemapBytes {
			return nil, "", report, failure(ErrorCapacity, "sitemap_index_byte_limit", "sitemap", "index exceeds sitemap byte limit")
		}
		if err := writeString(indexWriter, entry); err != nil {
			return nil, "", report, err
		}
	}
	if err := writeString(indexWriter, "</sitemapindex>\n"); err != nil {
		return nil, "", report, err
	}
	if err := indexWriter.Close(); err != nil {
		return nil, "", report, targetError("target_close_failed", true, err)
	}
	artifacts := make([]Artifact, 0, len(parts)+1)
	for _, part := range parts {
		artifacts = append(artifacts, part.artifact)
	}
	artifacts = append(artifacts, indexWriter.artifact(route, "application/xml; charset=utf-8"))
	return artifacts, "/" + route, report, nil
}

func encodeSitemapEntry(entry SitemapEntry) string {
	var builder strings.Builder
	builder.WriteString("  <url><loc>")
	builder.WriteString(xmlText(entry.Location))
	builder.WriteString("</loc>")
	if entry.LastModified != nil {
		builder.WriteString("<lastmod>")
		builder.WriteString(entry.LastModified.UTC().Format(time.RFC3339))
		builder.WriteString("</lastmod>")
	}
	for _, alternate := range entry.Alternates {
		builder.WriteString(`<xhtml:link rel="alternate" hreflang="`)
		builder.WriteString(xmlText(alternate.Locale))
		builder.WriteString(`" href="`)
		builder.WriteString(xmlText(alternate.Location))
		builder.WriteString(`"/>`)
	}
	builder.WriteString("</url>\n")
	return builder.String()
}

var _ io.Writer = (*measuredWriter)(nil)
