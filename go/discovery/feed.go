package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

type projectedFeedEntry struct {
	id          string
	link        string
	title       string
	summary     string
	contentHTML string
	publishedAt time.Time
	updatedAt   time.Time
	authors     []Person
}

func (module *Module) publishFeed(
	ctx context.Context,
	plan FeedPlan,
	sources Sources,
	transaction PublicationWriter,
) (Artifact, Report, error) {
	if strings.TrimSpace(plan.ID) == "" {
		return Artifact{}, Report{}, failure(ErrorContract, "feed_id_required", "id", "is required")
	}
	if plan.Format != FeedRSS && plan.Format != FeedAtom {
		return Artifact{}, Report{}, failure(ErrorContract, "unknown_feed_format", "format", "must be rss or atom")
	}
	if plan.UpdatedAt.IsZero() {
		return Artifact{}, Report{}, failure(ErrorContract, "feed_updated_at_required", "updatedAt", "is required for deterministic output")
	}
	route, err := normalizeArtifactRoute(plan.Route, "", "route")
	if err != nil {
		return Artifact{}, Report{}, err
	}
	source, err := module.source(plan.Source, sources, "source")
	if err != nil {
		return Artifact{}, Report{}, err
	}
	maxEntries := plan.MaxEntries
	if maxEntries == 0 {
		maxEntries = module.limits.MaxFeedEntries
	}
	if maxEntries < 1 || maxEntries > module.limits.MaxFeedEntries {
		return Artifact{}, Report{}, failure(ErrorCapacity, "feed_entry_limit", "maxEntries", "must be between 1 and %d", module.limits.MaxFeedEntries)
	}
	entries := make([]projectedFeedEntry, 0, min(maxEntries, module.limits.MaxSourceBatch))
	report := Report{Diagnostics: []Diagnostic{}}
	sanitizer := bluemonday.UGCPolicy()
	err = module.scan(ctx, source, maxEntries, func(record Record) error {
		if record.Feed == nil {
			return failure(ErrorContract, "feed_facts_required", "record.feed", "is required for a feed source")
		}
		projection, pageReport, err := module.Project(record.Page)
		report.Diagnostics = append(report.Diagnostics, pageReport.Diagnostics...)
		if err != nil {
			return err
		}
		if projection.Sitemap == nil {
			return nil
		}
		if record.SortKey != projection.CanonicalURL {
			return failure(ErrorConflict, "source_sort_key_mismatch", "record.sortKey", "must equal the canonical URL for feed sources")
		}
		facts := record.Feed
		if strings.TrimSpace(facts.ID) == "" || strings.TrimSpace(facts.Title) == "" || facts.PublishedAt.IsZero() {
			return failure(ErrorContract, "incomplete_feed_entry", "record.feed", "id, title and publishedAt are required")
		}
		updatedAt := facts.PublishedAt
		if facts.ModifiedAt != nil {
			updatedAt = *facts.ModifiedAt
		}
		entries = append(entries, projectedFeedEntry{
			id: facts.ID, link: projection.CanonicalURL,
			title: facts.Title, summary: facts.Summary,
			contentHTML: sanitizer.Sanitize(facts.ContentHTML),
			publishedAt: facts.PublishedAt.UTC(), updatedAt: updatedAt.UTC(),
			authors: facts.Authors,
		})
		return nil
	})
	if err != nil {
		return Artifact{}, report, err
	}
	writer, err := createArtifact(ctx, transaction, route, "application/xml; charset=utf-8")
	if err != nil {
		return Artifact{}, report, err
	}
	feedURL, err := module.absoluteURL("/"+route, "route")
	if err != nil {
		return Artifact{}, report, err
	}
	switch plan.Format {
	case FeedRSS:
		err = module.writeRSS(writer, plan, feedURL, entries)
	case FeedAtom:
		err = module.writeAtom(writer, plan, feedURL, entries)
	}
	if err != nil {
		return Artifact{}, report, err
	}
	if err := writer.Close(); err != nil {
		return Artifact{}, report, targetError("target_close_failed", true, err)
	}
	return writer.artifact(route, "application/xml; charset=utf-8"), report, nil
}

func (module *Module) writeRSS(
	writer *measuredWriter,
	plan FeedPlan,
	feedURL string,
	entries []projectedFeedEntry,
) error {
	language := plan.Language
	if language == "" {
		language = module.site.DefaultLocale
	}
	header := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:content="http://purl.org/rss/1.0/modules/content/">` + "\n" +
		"<channel><title>" + xmlText(plan.Title) + "</title>" +
		"<link>" + xmlText(module.origin.String()+"/") + "</link>" +
		"<description>" + xmlText(plan.Description) + "</description>" +
		"<language>" + xmlText(language) + "</language>" +
		"<lastBuildDate>" + plan.UpdatedAt.UTC().Format(time.RFC1123Z) + "</lastBuildDate>" +
		`<atom:link href="` + xmlText(feedURL) + `" rel="self" type="application/rss+xml"/>` + "\n"
	if err := writeString(writer, header); err != nil {
		return err
	}
	for _, entry := range entries {
		value := "<item><title>" + xmlText(entry.title) + "</title>" +
			"<link>" + xmlText(entry.link) + "</link>" +
			`<guid isPermaLink="false">` + xmlText(entry.id) + "</guid>" +
			"<pubDate>" + entry.publishedAt.Format(time.RFC1123Z) + "</pubDate>" +
			"<description>" + xmlText(entry.summary) + "</description>"
		for _, author := range entry.authors {
			value += "<author>" + xmlText(author.Name) + "</author>"
		}
		if entry.contentHTML != "" {
			value += "<content:encoded>" + xmlText(entry.contentHTML) + "</content:encoded>"
		}
		value += "</item>\n"
		if err := writeString(writer, value); err != nil {
			return err
		}
	}
	return writeString(writer, "</channel></rss>\n")
}

func (module *Module) writeAtom(
	writer *measuredWriter,
	plan FeedPlan,
	feedURL string,
	entries []projectedFeedEntry,
) error {
	header := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<feed xmlns="http://www.w3.org/2005/Atom">` +
		"<id>" + xmlText(plan.ID) + "</id><title>" + xmlText(plan.Title) + "</title>" +
		"<subtitle>" + xmlText(plan.Description) + "</subtitle>" +
		"<updated>" + plan.UpdatedAt.UTC().Format(time.RFC3339) + "</updated>" +
		`<link rel="self" type="application/atom+xml" href="` + xmlText(feedURL) + `"/>` +
		`<link rel="alternate" href="` + xmlText(module.origin.String()+"/") + `"/>` + "\n"
	if err := writeString(writer, header); err != nil {
		return err
	}
	for _, entry := range entries {
		value := "<entry><id>" + xmlText(entry.id) + "</id><title>" + xmlText(entry.title) + "</title>" +
			"<published>" + entry.publishedAt.Format(time.RFC3339) + "</published>" +
			"<updated>" + entry.updatedAt.Format(time.RFC3339) + "</updated>" +
			`<link rel="alternate" href="` + xmlText(entry.link) + `"/>` +
			"<summary>" + xmlText(entry.summary) + "</summary>"
		for _, author := range entry.authors {
			value += "<author><name>" + xmlText(author.Name) + "</name>"
			if author.URL != "" {
				authorURL, err := module.absoluteURL(author.URL, "feed.author.url")
				if err != nil {
					return err
				}
				value += "<uri>" + xmlText(authorURL) + "</uri>"
			}
			value += "</author>"
		}
		if entry.contentHTML != "" {
			value += `<content type="html">` + xmlText(entry.contentHTML) + "</content>"
		}
		value += "</entry>\n"
		if err := writeString(writer, value); err != nil {
			return err
		}
	}
	return writeString(writer, "</feed>\n")
}

var _ = fmt.Sprintf
