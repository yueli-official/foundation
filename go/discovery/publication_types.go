package discovery

import (
	"context"
	"io"
	"time"
)

type SourceID string
type Cursor string

type FeedEntryFacts struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary,omitempty"`
	ContentHTML string     `json:"contentHtml,omitempty"`
	PublishedAt time.Time  `json:"publishedAt"`
	ModifiedAt  *time.Time `json:"modifiedAt,omitempty"`
	Authors     []Person   `json:"authors,omitempty"`
}

type Record struct {
	SortKey string          `json:"sortKey"`
	Page    PageDescriptor  `json:"page"`
	Feed    *FeedEntryFacts `json:"feed,omitempty"`
}

type Batch struct {
	Records    []Record `json:"records"`
	NextCursor Cursor   `json:"nextCursor,omitempty"`
	Done       bool     `json:"done"`
}

type CursorSource interface {
	Next(context.Context, Cursor, int) (Batch, error)
}

type Sources map[SourceID]CursorSource

type SitemapPlan struct {
	Source SourceID `json:"source"`
	Route  string   `json:"route,omitempty"`
}

type FeedFormat string

const (
	FeedRSS  FeedFormat = "rss"
	FeedAtom FeedFormat = "atom"
)

type FeedPlan struct {
	ID          string     `json:"id"`
	Source      SourceID   `json:"source"`
	Format      FeedFormat `json:"format"`
	Route       string     `json:"route"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Language    string     `json:"language,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	MaxEntries  int        `json:"maxEntries,omitempty"`
}

type RobotsGroup struct {
	UserAgents []string `json:"userAgents"`
	Allow      []string `json:"allow,omitempty"`
	Disallow   []string `json:"disallow,omitempty"`
}

type RobotsPlan struct {
	Route    string        `json:"route,omitempty"`
	Groups   []RobotsGroup `json:"groups,omitempty"`
	Sitemaps []string      `json:"sitemaps,omitempty"`
}

type PublicationPlan struct {
	Sitemap *SitemapPlan `json:"sitemap,omitempty"`
	Feeds   []FeedPlan   `json:"feeds,omitempty"`
	Robots  *RobotsPlan  `json:"robots,omitempty"`
}

type Artifact struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

type PublicationManifest struct {
	ContractVersion string     `json:"contractVersion"`
	Artifacts       []Artifact `json:"artifacts"`
}

type PublicationWriter interface {
	Create(context.Context, string, string) (io.WriteCloser, error)
	Commit(context.Context, PublicationManifest) error
	Abort(context.Context, error) error
}

type PublishTarget interface {
	Begin(context.Context) (PublicationWriter, error)
}
