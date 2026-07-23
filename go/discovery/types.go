package discovery

import (
	"encoding/json"
	"time"
)

const ContractVersion = "discovery.v1"

type SiteProfile struct {
	Origin         string `json:"origin"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	DefaultLocale  string `json:"defaultLocale"`
	DefaultImage   *Image `json:"defaultImage,omitempty"`
	OrganizationID string `json:"organizationId,omitempty"`
}

type URLPolicy struct {
	PreserveQuery bool `json:"preserveQuery,omitempty"`
	TrailingSlash bool `json:"trailingSlash,omitempty"`
}

type Limits struct {
	MaxTitleBytes          int `json:"maxTitleBytes,omitempty"`
	MaxDescriptionBytes    int `json:"maxDescriptionBytes,omitempty"`
	MaxPageJSONBytes       int `json:"maxPageJsonBytes,omitempty"`
	MaxStructuredDataNodes int `json:"maxStructuredDataNodes,omitempty"`
	MaxSourceBatch         int `json:"maxSourceBatch,omitempty"`
	MaxFeedEntries         int `json:"maxFeedEntries,omitempty"`
	MaxSitemapURLs         int `json:"maxSitemapUrls,omitempty"`
	MaxSitemapBytes        int `json:"maxSitemapBytes,omitempty"`
	MaxSitemapParts        int `json:"maxSitemapParts,omitempty"`
	MaxRobotsBytes         int `json:"maxRobotsBytes,omitempty"`
}

type Definition struct {
	ContractVersion string      `json:"contractVersion"`
	Site            SiteProfile `json:"site"`
	URLPolicy       URLPolicy   `json:"urlPolicy,omitempty"`
	Limits          Limits      `json:"limits,omitempty"`
}

type Visibility string

const (
	Discoverable Visibility = "discoverable"
	Unlisted     Visibility = "unlisted"
	Private      Visibility = "private"
)

type FollowPolicy string

const (
	Follow   FollowPolicy = "follow"
	NoFollow FollowPolicy = "nofollow"
)

type Image struct {
	URL    string `json:"url"`
	Alt    string `json:"alt,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Type   string `json:"type,omitempty"`
}

type Person struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type WebPage struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Image       *Image     `json:"image,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	ModifiedAt  *time.Time `json:"modifiedAt,omitempty"`
}

type Article struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Image       *Image     `json:"image,omitempty"`
	PublishedAt time.Time  `json:"publishedAt"`
	ModifiedAt  *time.Time `json:"modifiedAt,omitempty"`
	Authors     []Person   `json:"authors,omitempty"`
	Section     string     `json:"section,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
}

type Collection struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Image       *Image `json:"image,omitempty"`
}

type Product struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Image         *Image `json:"image,omitempty"`
	SKU           string `json:"sku,omitempty"`
	Brand         string `json:"brand,omitempty"`
	Price         string `json:"price,omitempty"`
	PriceCurrency string `json:"priceCurrency,omitempty"`
	Availability  string `json:"availability,omitempty"`
}

type SubjectKind string

const (
	SubjectWebPage    SubjectKind = "web_page"
	SubjectArticle    SubjectKind = "article"
	SubjectCollection SubjectKind = "collection"
	SubjectProduct    SubjectKind = "product"
)

type Subject struct {
	Kind       SubjectKind `json:"kind"`
	WebPage    *WebPage    `json:"webPage,omitempty"`
	Article    *Article    `json:"article,omitempty"`
	Collection *Collection `json:"collection,omitempty"`
	Product    *Product    `json:"product,omitempty"`
}

func WebPageSubject(value WebPage) Subject {
	return Subject{Kind: SubjectWebPage, WebPage: &value}
}

func ArticleSubject(value Article) Subject {
	return Subject{Kind: SubjectArticle, Article: &value}
}

func CollectionSubject(value Collection) Subject {
	return Subject{Kind: SubjectCollection, Collection: &value}
}

func ProductSubject(value Product) Subject {
	return Subject{Kind: SubjectProduct, Product: &value}
}

type Alternate struct {
	Locale string `json:"locale"`
	Path   string `json:"path"`
}

type Breadcrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type PageDescriptor struct {
	Key         string       `json:"key"`
	Path        string       `json:"path"`
	Locale      string       `json:"locale,omitempty"`
	Visibility  Visibility   `json:"visibility,omitempty"`
	Follow      FollowPolicy `json:"follow,omitempty"`
	ModifiedAt  *time.Time   `json:"modifiedAt,omitempty"`
	Subject     Subject      `json:"subject"`
	Alternates  []Alternate  `json:"alternates,omitempty"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs,omitempty"`
}

type LinkTag struct {
	Rel      string `json:"rel"`
	Href     string `json:"href"`
	HrefLang string `json:"hreflang,omitempty"`
}

type MetaTag struct {
	Name     string `json:"name,omitempty"`
	Property string `json:"property,omitempty"`
	Content  string `json:"content"`
}

type StructuredData struct {
	ID   string          `json:"id"`
	JSON json.RawMessage `json:"json"`
}

type HeadProjection struct {
	Title          string           `json:"title"`
	Links          []LinkTag        `json:"links"`
	Meta           []MetaTag        `json:"meta"`
	StructuredData []StructuredData `json:"structuredData"`
}

type HeaderProjection struct {
	Link       []string `json:"link,omitempty"`
	XRobotsTag string   `json:"xRobotsTag,omitempty"`
}

type SitemapEntry struct {
	Location     string         `json:"location"`
	LastModified *time.Time     `json:"lastModified,omitempty"`
	Alternates   []AlternateURL `json:"alternates,omitempty"`
}

type AlternateURL struct {
	Locale   string `json:"locale"`
	Location string `json:"location"`
}

type PageProjection struct {
	ContractVersion string           `json:"contractVersion"`
	Key             string           `json:"key"`
	CanonicalURL    string           `json:"canonicalUrl"`
	Head            HeadProjection   `json:"head"`
	Headers         HeaderProjection `json:"headers"`
	Sitemap         *SitemapEntry    `json:"sitemap,omitempty"`
	Diagnostics     []Diagnostic     `json:"diagnostics"`
}
