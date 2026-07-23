package urllifecycle

import (
	"context"
	"io"
	"time"
)

type ResourceKind string
type Namespace string
type CommandID string
type Revision uint64
type RouteRevision uint64
type Digest string

type ResourceKey struct {
	Kind ResourceKind `json:"kind"`
	ID   string       `json:"id"`
}

type RouteKey struct {
	Resource ResourceKey `json:"resource"`
	Variant  string      `json:"variant,omitempty"`
}

type QueryValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// LocalRef is an origin-relative public URL identity. Query contains only the
// identity dimensions registered by the matching Namespace.
type LocalRef struct {
	Path  string       `json:"path"`
	Query []QueryValue `json:"query,omitempty"`
}

type Lookup struct {
	EscapedPath string
	RawQuery    string
}

type ActorRef struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
}

type RedirectMode string

const (
	PermanentLegacy         RedirectMode = "permanent_legacy"
	TemporaryLegacy         RedirectMode = "temporary_legacy"
	TemporaryPreserveMethod RedirectMode = "temporary_preserve_method"
	PermanentPreserveMethod RedirectMode = "permanent_preserve_method"
)

func (mode RedirectMode) StatusCode() int {
	switch mode {
	case PermanentLegacy:
		return 301
	case TemporaryLegacy:
		return 302
	case TemporaryPreserveMethod:
		return 307
	case PermanentPreserveMethod:
		return 308
	default:
		return 0
	}
}

func (mode RedirectMode) Permanent() bool {
	return mode == PermanentLegacy || mode == PermanentPreserveMethod
}

type QueryMode string

const (
	QueryCanonicalWithExtras QueryMode = "canonical_with_extras"
	QueryPreserve            QueryMode = "preserve"
	QueryDrop                QueryMode = "drop"
	QueryReplace             QueryMode = "replace"
)

type RedirectPolicy struct {
	Mode         RedirectMode `json:"mode"`
	Query        QueryMode    `json:"query"`
	ReplaceQuery string       `json:"replaceQuery,omitempty"`
}

type TargetKind string

const (
	TargetRoute    TargetKind = "route"
	TargetExternal TargetKind = "external"
)

type Target struct {
	Kind     TargetKind `json:"kind"`
	Route    RouteKey   `json:"route,omitempty"`
	External string     `json:"external,omitempty"`
}

func RouteTarget(route RouteKey) Target {
	return Target{Kind: TargetRoute, Route: route}
}

func ExternalTarget(rawURL string) Target {
	return Target{Kind: TargetExternal, External: rawURL}
}

type Alias struct {
	Ref    LocalRef       `json:"ref"`
	Policy RedirectPolicy `json:"policy"`
}

type ActiveRoute struct {
	Canonical LocalRef `json:"canonical"`
	Aliases   []Alias  `json:"aliases,omitempty"`
}

type FormerKind string

const (
	FormerRedirectToCurrent FormerKind = "redirect_to_current"
	FormerRedirectToRoute   FormerKind = "redirect_to_route"
	FormerGone              FormerKind = "gone"
	FormerRelease           FormerKind = "release"
)

type FormerOutcome struct {
	Kind     FormerKind     `json:"kind"`
	Target   RouteKey       `json:"target,omitempty"`
	Redirect RedirectPolicy `json:"redirect,omitempty"`
}

type DeparturePolicy struct {
	Canonical FormerOutcome `json:"canonical"`
	Aliases   FormerOutcome `json:"aliases"`
}

type ResourceChange struct {
	Route            RouteKey        `json:"route"`
	ExpectedRevision RouteRevision   `json:"expectedRevision,omitempty"`
	Desired          *ActiveRoute    `json:"desired,omitempty"`
	Departures       DeparturePolicy `json:"departures"`
}

type TemporaryRedirect struct {
	Target    Target         `json:"target"`
	Policy    RedirectPolicy `json:"policy"`
	ExpiresAt *time.Time     `json:"expiresAt,omitempty"`
}

type OverlayChange struct {
	Owner            RouteKey           `json:"owner"`
	Source           LocalRef           `json:"source"`
	ExpectedRevision RouteRevision      `json:"expectedRevision,omitempty"`
	Desired          *TemporaryRedirect `json:"desired,omitempty"`
}

type ChangeSet struct {
	CommandID       CommandID        `json:"commandId"`
	Actor           ActorRef         `json:"actor,omitempty"`
	Reason          string           `json:"reason"`
	ExpectedHead    Revision         `json:"expectedHead,omitempty"`
	ResourceChanges []ResourceChange `json:"resourceChanges,omitempty"`
	OverlayChanges  []OverlayChange  `json:"overlayChanges,omitempty"`
}

type MutationMeta struct {
	CommandID    CommandID
	Actor        ActorRef
	Reason       string
	ExpectedHead Revision
}

type Plan struct {
	Valid        bool         `json:"valid"`
	BaseRevision Revision     `json:"baseRevision"`
	IntentDigest Digest       `json:"intentDigest"`
	Effects      []Effect     `json:"effects"`
	Diagnostics  []Diagnostic `json:"diagnostics,omitempty"`
}

type PlanGuard struct {
	BaseRevision Revision
	IntentDigest Digest
}

type ApplyOptions struct {
	Guard *PlanGuard
}

type RouteRevisionResult struct {
	Route    RouteKey      `json:"route"`
	Revision RouteRevision `json:"revision"`
}

type Receipt struct {
	CommandID      CommandID             `json:"commandId"`
	IntentDigest   Digest                `json:"intentDigest"`
	Revision       Revision              `json:"revision"`
	RouteRevisions []RouteRevisionResult `json:"routeRevisions"`
	Effects        []Effect              `json:"effects"`
	Replay         bool                  `json:"replay"`
}

type EffectKind string

const (
	EffectClaim       EffectKind = "claim"
	EffectAlias       EffectKind = "alias"
	EffectRedirect    EffectKind = "redirect"
	EffectGone        EffectKind = "gone"
	EffectRelease     EffectKind = "release"
	EffectOverlaySet  EffectKind = "overlay_set"
	EffectOverlayDrop EffectKind = "overlay_drop"
)

type Effect struct {
	Kind   EffectKind `json:"kind"`
	Ref    LocalRef   `json:"ref"`
	Route  *RouteKey  `json:"route,omitempty"`
	Target *Target    `json:"target,omitempty"`
}

type Diagnostic struct {
	Code    string    `json:"code"`
	Field   string    `json:"field,omitempty"`
	Message string    `json:"message"`
	Ref     *LocalRef `json:"ref,omitempty"`
}

type ResolutionKind string

const (
	ResolutionCanonical ResolutionKind = "canonical"
	ResolutionAlias     ResolutionKind = "alias"
	ResolutionRedirect  ResolutionKind = "redirect"
	ResolutionGone      ResolutionKind = "gone"
	ResolutionUnknown   ResolutionKind = "unknown"
)

type Resolution struct {
	Kind       ResolutionKind `json:"kind"`
	Requested  LocalRef       `json:"requested"`
	Route      *RouteKey      `json:"route,omitempty"`
	Canonical  *LocalRef      `json:"canonical,omitempty"`
	Location   string         `json:"location,omitempty"`
	StatusCode int            `json:"statusCode,omitempty"`
	Revision   Revision       `json:"revision"`
	ChangedAt  time.Time      `json:"changedAt,omitempty"`
	ExpiresAt  *time.Time     `json:"expiresAt,omitempty"`
}

type Resolver interface {
	Resolve(context.Context, Lookup) (Resolution, error)
}

type Planner interface {
	Preview(context.Context, ChangeSet) (Plan, error)
}

type Transitioner interface {
	Apply(context.Context, ChangeSet, ApplyOptions) (Receipt, error)
}

type InspectQuery struct {
	Route *RouteKey
	Ref   *LocalRef
}

type Inspection struct {
	Route      *RouteKey          `json:"route,omitempty"`
	Active     *ActiveRoute       `json:"active,omitempty"`
	Resolution Resolution         `json:"resolution"`
	Overlay    *TemporaryRedirect `json:"overlay,omitempty"`
	Revision   RouteRevision      `json:"revision,omitempty"`
}

type ListQuery struct {
	Prefix string
	After  string
	Limit  int
}

type InspectionPage struct {
	Items []Inspection `json:"items"`
	Next  string       `json:"next,omitempty"`
}

type HistoryQuery struct {
	Route         *RouteKey
	AfterRevision Revision
	Limit         int
}

type TransitionSummary struct {
	CommandID CommandID `json:"commandId"`
	Revision  Revision  `json:"revision"`
	Actor     ActorRef  `json:"actor,omitempty"`
	Reason    string    `json:"reason"`
	AppliedAt time.Time `json:"appliedAt"`
}

type HistoryPage struct {
	Items []TransitionSummary `json:"items"`
	Next  Revision            `json:"next,omitempty"`
}

type Reader interface {
	Inspect(context.Context, InspectQuery) (Inspection, error)
	List(context.Context, ListQuery) (InspectionPage, error)
	History(context.Context, HistoryQuery) (HistoryPage, error)
}

type ExportQuery struct {
	IncludeAudit bool
}

type ArchiveManifest struct {
	FormatVersion uint64   `json:"formatVersion"`
	PolicyDigest  Digest   `json:"policyDigest"`
	Revision      Revision `json:"revision"`
	Records       uint64   `json:"records"`
	Digest        Digest   `json:"digest"`
}

type ArchiveReport struct {
	Valid       bool            `json:"valid"`
	Manifest    ArchiveManifest `json:"manifest"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

type RestoreCommand struct {
	DryRun       bool
	RequireEmpty bool
	CommandID    CommandID
	Actor        ActorRef
	Reason       string
}

type RestoreReport struct {
	Plan     Plan            `json:"plan"`
	Receipt  *Receipt        `json:"receipt,omitempty"`
	Manifest ArchiveManifest `json:"manifest"`
}

type RebuildCommand struct {
	DryRun       bool
	ExpectedHead Revision
}

type RebuildReport struct {
	Revision Revision `json:"revision"`
	Records  uint64   `json:"records"`
	Changed  bool     `json:"changed"`
}

type Archiver interface {
	Export(context.Context, ExportQuery, io.Writer) (ArchiveManifest, error)
	VerifyArchive(context.Context, io.Reader) (ArchiveReport, error)
	Restore(context.Context, RestoreCommand, io.Reader) (RestoreReport, error)
	RebuildProjection(context.Context, RebuildCommand) (RebuildReport, error)
}

type Module interface {
	Resolver
	Planner
	Transitioner
	Reader
	Archiver
}
