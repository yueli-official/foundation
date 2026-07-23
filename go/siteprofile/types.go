package siteprofile

import (
	"context"
	"io"
	"time"
)

type Revision uint64
type Digest string

type VisualKind string

const (
	VisualIcon  VisualKind = "icon"
	VisualAsset VisualKind = "asset"
)

type Visual struct {
	Kind VisualKind `json:"kind"`
	Ref  string     `json:"ref"`
	Alt  string     `json:"alt,omitempty"`
}

type Identity struct {
	Name        string `json:"name"`
	Tagline     string `json:"tagline,omitempty"`
	Description string `json:"description,omitempty"`
}

type Branding struct {
	Logo     *Visual `json:"logo,omitempty"`
	DarkLogo *Visual `json:"darkLogo,omitempty"`
	Favicon  *Visual `json:"favicon,omitempty"`
}

type Link struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Href  string `json:"href"`
	Icon  string `json:"icon,omitempty"`
}

type AnnouncementTone string

const (
	AnnouncementNeutral  AnnouncementTone = "neutral"
	AnnouncementInfo     AnnouncementTone = "info"
	AnnouncementSuccess  AnnouncementTone = "success"
	AnnouncementWarning  AnnouncementTone = "warning"
	AnnouncementCritical AnnouncementTone = "critical"
)

type Announcement struct {
	Enabled     bool             `json:"enabled"`
	Text        string           `json:"text,omitempty"`
	Tone        AnnouncementTone `json:"tone,omitempty"`
	Action      *Link            `json:"action,omitempty"`
	Dismissible bool             `json:"dismissible"`
	StartsAt    *time.Time       `json:"startsAt,omitempty"`
	EndsAt      *time.Time       `json:"endsAt,omitempty"`
}

type ContactKind string

const (
	ContactEmail ContactKind = "email"
	ContactPhone ContactKind = "phone"
	ContactLink  ContactKind = "link"
	ContactText  ContactKind = "text"
)

type Contact struct {
	ID    string      `json:"id"`
	Kind  ContactKind `json:"kind"`
	Label string      `json:"label"`
	Value string      `json:"value"`
	Icon  string      `json:"icon,omitempty"`
}

type Support struct {
	Contacts []Contact `json:"contacts"`
}

type LinkGroup struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Links []Link `json:"links"`
}

type SocialLink struct {
	ID       string `json:"id"`
	Platform string `json:"platform"`
	Label    string `json:"label,omitempty"`
	URL      string `json:"url"`
	Icon     string `json:"icon,omitempty"`
}

type ComplianceRecord struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Number string `json:"number"`
	URL    string `json:"url,omitempty"`
}

type Compliance struct {
	Records   []ComplianceRecord `json:"records"`
	ExtraText string             `json:"extraText,omitempty"`
}

type Footer struct {
	Tagline    string       `json:"tagline,omitempty"`
	Copyright  string       `json:"copyright,omitempty"`
	LinkGroups []LinkGroup  `json:"linkGroups"`
	Social     []SocialLink `json:"social"`
	Legal      []Link       `json:"legal"`
	Compliance Compliance   `json:"compliance"`
}

type Profile struct {
	Identity     Identity     `json:"identity"`
	Branding     Branding     `json:"branding"`
	Announcement Announcement `json:"announcement"`
	Support      Support      `json:"support"`
	Footer       Footer       `json:"footer"`
}

type Snapshot struct {
	Profile        Profile   `json:"profile"`
	Revision       Revision  `json:"revision"`
	ETag           string    `json:"etag"`
	DocumentDigest Digest    `json:"documentDigest"`
	SchemaVersion  uint64    `json:"schemaVersion"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ReplaceCommand struct {
	ExpectedRevision Revision `json:"expectedRevision"`
	Profile          Profile  `json:"profile"`
}

type ReplaceResult struct {
	Snapshot Snapshot `json:"snapshot"`
	Changed  bool     `json:"changed"`
}

type PublicProjection struct {
	Snapshot     Snapshot   `json:"snapshot"`
	NextChangeAt *time.Time `json:"nextChangeAt,omitempty"`
}

type ArchiveManifest struct {
	FormatVersion  uint64    `json:"formatVersion"`
	SchemaVersion  uint64    `json:"schemaVersion"`
	Revision       Revision  `json:"revision"`
	DocumentDigest Digest    `json:"documentDigest"`
	ArchiveDigest  Digest    `json:"archiveDigest"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ArchiveReport struct {
	Valid       bool            `json:"valid"`
	Manifest    ArchiveManifest `json:"manifest"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

type RestoreCommand struct {
	ExpectedRevision Revision `json:"expectedRevision"`
	DryRun           bool     `json:"dryRun"`
}

type RestoreResult struct {
	Manifest ArchiveManifest `json:"manifest"`
	Result   *ReplaceResult  `json:"result,omitempty"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type FieldControl string

const (
	ControlText     FieldControl = "text"
	ControlTextarea FieldControl = "textarea"
	ControlToggle   FieldControl = "toggle"
	ControlSelect   FieldControl = "select"
	ControlVisual   FieldControl = "visual"
	ControlDateTime FieldControl = "datetime"
	ControlList     FieldControl = "list"
)

type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type FormField struct {
	Path        string        `json:"path"`
	Label       string        `json:"label"`
	Description string        `json:"description,omitempty"`
	Control     FieldControl  `json:"control"`
	Required    bool          `json:"required"`
	MaxLength   int           `json:"maxLength,omitempty"`
	MaxItems    int           `json:"maxItems,omitempty"`
	Options     []FieldOption `json:"options,omitempty"`
	ItemFields  []FormField   `json:"itemFields,omitempty"`
}

type FormSection struct {
	ID          string      `json:"id"`
	Label       string      `json:"label"`
	Description string      `json:"description,omitempty"`
	Fields      []FormField `json:"fields"`
}

type FormSchema struct {
	Version  uint64        `json:"version"`
	Digest   Digest        `json:"digest"`
	Sections []FormSection `json:"sections"`
}

type Reader interface {
	Get(context.Context) (Snapshot, error)
}

type Replacer interface {
	Replace(context.Context, ReplaceCommand) (ReplaceResult, error)
}

type Describer interface {
	Schema() FormSchema
}

type Projector interface {
	PublicAt(context.Context, time.Time) (PublicProjection, error)
}

type Archiver interface {
	Export(context.Context, io.Writer) (ArchiveManifest, error)
	VerifyArchive(io.Reader) (ArchiveReport, error)
	Restore(context.Context, RestoreCommand, io.Reader) (RestoreResult, error)
}

type Module interface {
	Reader
	Replacer
	Describer
	Projector
	Archiver
}
