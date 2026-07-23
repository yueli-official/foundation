package urllifecycle

import (
	"context"
	"database/sql"
	"net/url"
	"time"
)

type sqlRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (adapter *PostgresAdapter) resolvePostgres(
	ctx context.Context,
	queryer sqlRowQueryer,
	lookup normalizedLookup,
	now time.Time,
) (Resolution, error) {
	var revision Revision
	if err := queryer.QueryRowContext(ctx, `
SELECT revision
FROM `+adapter.table("instances")+`
WHERE instance_key = $1`, adapter.instanceKey).Scan(&revision); err != nil {
		return Resolution{}, unavailable("load resolve revision", err)
	}
	ref, err := adapter.loadReferenceByID(ctx, queryer, storageRefID(lookup.ref), lookup.ref)
	if err == sql.ErrNoRows {
		return Resolution{Kind: ResolutionUnknown, Requested: lookup.ref.ref, Revision: revision}, nil
	}
	if err != nil {
		return Resolution{}, err
	}
	overlay, overlayErr := adapter.loadOverlayByID(ctx, queryer, storageRefID(lookup.ref), lookup.ref)
	if overlayErr != nil && overlayErr != sql.ErrNoRows {
		return Resolution{}, overlayErr
	}
	if overlayErr == nil &&
		(overlay.Redirect.ExpiresAt == nil || now.Before(overlay.Redirect.ExpiresAt.UTC())) {
		location, canonical, route, err := adapter.postgresTargetLocation(
			ctx, queryer, lookup, overlay.Redirect.Target, overlay.Redirect.Policy,
		)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Kind: ResolutionRedirect, Requested: lookup.ref.ref, Route: route,
			Canonical: canonical, Location: location,
			StatusCode: overlay.Redirect.Policy.Mode.StatusCode(),
			Revision:   revision, ChangedAt: overlay.ChangedAt,
			ExpiresAt: overlay.Redirect.ExpiresAt,
		}, nil
	}
	switch ref.Kind {
	case referenceCanonical:
		route := ref.Owner
		canonical := ref.Ref.ref
		return Resolution{
			Kind: ResolutionCanonical, Requested: lookup.ref.ref, Route: &route,
			Canonical: &canonical, Revision: revision, ChangedAt: ref.ChangedAt,
		}, nil
	case referenceAlias:
		location, canonical, route, err := adapter.postgresTargetLocation(
			ctx, queryer, lookup, RouteTarget(ref.Owner), ref.Policy,
		)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Kind: ResolutionAlias, Requested: lookup.ref.ref, Route: route,
			Canonical: canonical, Location: location, StatusCode: ref.Policy.Mode.StatusCode(),
			Revision: revision, ChangedAt: ref.ChangedAt,
		}, nil
	case referenceRedirect:
		location, canonical, route, err := adapter.postgresTargetLocation(
			ctx, queryer, lookup, ref.Target, ref.Policy,
		)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{
			Kind: ResolutionRedirect, Requested: lookup.ref.ref, Route: route,
			Canonical: canonical, Location: location, StatusCode: ref.Policy.Mode.StatusCode(),
			Revision: revision, ChangedAt: ref.ChangedAt,
		}, nil
	case referenceGone:
		route := ref.Owner
		return Resolution{
			Kind: ResolutionGone, Requested: lookup.ref.ref, Route: &route,
			Revision: revision, ChangedAt: ref.ChangedAt,
		}, nil
	default:
		return Resolution{}, &Error{Kind: ErrorCorruptState, Field: "postgres.reference", Message: "has an unknown outcome"}
	}
}

func (adapter *PostgresAdapter) loadReferenceByID(
	ctx context.Context,
	queryer sqlRowQueryer,
	refID string,
	normalized normalizedRef,
) (storedReference, error) {
	var kind string
	var ownerKind, ownerID, ownerVariant string
	var targetKind, targetRouteKind, targetRouteID, targetRouteVariant, targetExternal string
	var redirectMode, queryMode, replaceQuery string
	var changedAt time.Time
	err := queryer.QueryRowContext(ctx, `
SELECT kind, owner_kind, owner_id, owner_variant,
       target_kind, target_route_kind, target_route_id, target_route_variant,
       target_external, redirect_mode, query_mode, replace_query, changed_at
FROM `+adapter.table("references")+`
WHERE instance_key = $1 AND ref_id = $2`,
		adapter.instanceKey, refID,
	).Scan(
		&kind, &ownerKind, &ownerID, &ownerVariant,
		&targetKind, &targetRouteKind, &targetRouteID, &targetRouteVariant,
		&targetExternal, &redirectMode, &queryMode, &replaceQuery, &changedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return storedReference{}, err
		}
		return storedReference{}, unavailable("load resolve reference", err)
	}
	return storedReference{
		Ref: normalized, Kind: referenceKind(kind),
		Owner: RouteKey{
			Resource: ResourceKey{Kind: ResourceKind(ownerKind), ID: ownerID},
			Variant:  ownerVariant,
		},
		Target: Target{
			Kind: TargetKind(targetKind),
			Route: RouteKey{
				Resource: ResourceKey{Kind: ResourceKind(targetRouteKind), ID: targetRouteID},
				Variant:  targetRouteVariant,
			},
			External: targetExternal,
		},
		Policy: RedirectPolicy{
			Mode: RedirectMode(redirectMode), Query: QueryMode(queryMode), ReplaceQuery: replaceQuery,
		},
		ChangedAt: changedAt.UTC(),
	}, nil
}

func (adapter *PostgresAdapter) loadOverlayByID(
	ctx context.Context,
	queryer sqlRowQueryer,
	refID string,
	source normalizedRef,
) (storedOverlay, error) {
	var ownerKind, ownerID, ownerVariant string
	var targetKind, targetRouteKind, targetRouteID, targetRouteVariant, targetExternal string
	var redirectMode, queryMode, replaceQuery string
	var expiresAt sql.NullTime
	var changedAt time.Time
	err := queryer.QueryRowContext(ctx, `
SELECT owner_kind, owner_id, owner_variant,
       target_kind, target_route_kind, target_route_id, target_route_variant,
       target_external, redirect_mode, query_mode, replace_query, expires_at, changed_at
FROM `+adapter.table("overlays")+`
WHERE instance_key = $1 AND ref_id = $2`,
		adapter.instanceKey, refID,
	).Scan(
		&ownerKind, &ownerID, &ownerVariant,
		&targetKind, &targetRouteKind, &targetRouteID, &targetRouteVariant,
		&targetExternal, &redirectMode, &queryMode, &replaceQuery,
		&expiresAt, &changedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return storedOverlay{}, err
		}
		return storedOverlay{}, unavailable("load resolve overlay", err)
	}
	redirect := TemporaryRedirect{
		Target: Target{
			Kind: TargetKind(targetKind),
			Route: RouteKey{
				Resource: ResourceKey{Kind: ResourceKind(targetRouteKind), ID: targetRouteID},
				Variant:  targetRouteVariant,
			},
			External: targetExternal,
		},
		Policy: RedirectPolicy{
			Mode: RedirectMode(redirectMode), Query: QueryMode(queryMode), ReplaceQuery: replaceQuery,
		},
	}
	if expiresAt.Valid {
		value := expiresAt.Time.UTC()
		redirect.ExpiresAt = &value
	}
	return storedOverlay{
		Owner: RouteKey{
			Resource: ResourceKey{Kind: ResourceKind(ownerKind), ID: ownerID},
			Variant:  ownerVariant,
		},
		Source: source, Redirect: redirect, ChangedAt: changedAt.UTC(),
	}, nil
}

func (adapter *PostgresAdapter) postgresTargetLocation(
	ctx context.Context,
	queryer sqlRowQueryer,
	lookup normalizedLookup,
	target Target,
	policy RedirectPolicy,
) (string, *LocalRef, *RouteKey, error) {
	switch target.Kind {
	case TargetRoute:
		var canonicalRefID string
		err := queryer.QueryRowContext(ctx, `
SELECT canonical_ref_id
FROM `+adapter.table("routes")+`
WHERE instance_key = $1
  AND resource_kind = $2
  AND resource_id = $3
  AND variant = $4`,
			adapter.instanceKey, target.Route.Resource.Kind,
			target.Route.Resource.ID, target.Route.Variant,
		).Scan(&canonicalRefID)
		if err != nil {
			return "", nil, nil, &Error{
				Kind: ErrorCorruptState, Field: "postgres.target",
				Message: "target route has no canonical", Cause: err,
			}
		}
		var path, renderedQuery string
		if err := queryer.QueryRowContext(ctx, `
SELECT path, rendered_query
FROM `+adapter.table("references")+`
WHERE instance_key = $1 AND ref_id = $2`,
			adapter.instanceKey, canonicalRefID,
		).Scan(&path, &renderedQuery); err != nil {
			return "", nil, nil, &Error{
				Kind: ErrorCorruptState, Field: "postgres.target",
				Message: "target canonical reference is missing", Cause: err,
			}
		}
		normalized, err := adapter.catalog.normalizeLookup(Lookup{
			EscapedPath: path, RawQuery: renderedQuery,
		})
		if err != nil {
			return "", nil, nil, &Error{
				Kind: ErrorCorruptState, Field: "postgres.target",
				Message: "target canonical reference is invalid", Cause: err,
			}
		}
		canonical := normalized.ref.ref
		route := target.Route
		return applyQueryPolicy(path, normalized.ref.query, lookup, policy), &canonical, &route, nil
	case TargetExternal:
		return externalTargetLocation(target.External, lookup, policy)
	default:
		return "", nil, nil, &Error{Kind: ErrorCorruptState, Field: "postgres.target", Message: "has an unknown kind"}
	}
}

func externalTargetLocation(
	raw string,
	lookup normalizedLookup,
	policy RedirectPolicy,
) (string, *LocalRef, *RouteKey, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", nil, nil, &Error{
			Kind: ErrorCorruptState, Field: "postgres.target",
			Message: "external target is invalid", Cause: err,
		}
	}
	baseQuery := parsed.RawQuery
	parsed.RawQuery = ""
	return applyQueryPolicy(parsed.String(), baseQuery, lookup, policy), nil, nil, nil
}
