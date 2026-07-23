package urllifecycle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"reflect"
)

func storageRefID(ref normalizedRef) string {
	sum := sha256.Sum256([]byte(ref.key))
	return hex.EncodeToString(sum[:])
}

func storageRouteID(route RouteKey) string {
	sum := sha256.Sum256([]byte(routeMapKey(route)))
	return hex.EncodeToString(sum[:])
}

func (adapter *PostgresAdapter) loadState(
	ctx context.Context,
	tx *sql.Tx,
	lock bool,
) (registryState, error) {
	state := emptyState()
	query := `SELECT revision FROM ` + adapter.table("instances") + ` WHERE instance_key = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	if err := tx.QueryRowContext(ctx, query, adapter.instanceKey).Scan(&state.Revision); err != nil {
		return registryState{}, unavailable("load instance revision", err)
	}

	refRows, err := tx.QueryContext(ctx, `
SELECT path, rendered_query, kind,
       owner_kind, owner_id, owner_variant,
       target_kind, target_route_kind, target_route_id, target_route_variant,
       target_external, redirect_mode, query_mode, replace_query, changed_at
FROM `+adapter.table("references")+`
WHERE instance_key = $1`, adapter.instanceKey)
	if err != nil {
		return registryState{}, unavailable("list references", err)
	}
	for refRows.Next() {
		var path, renderedQuery, kind string
		var ownerKind, ownerID, ownerVariant string
		var targetKind, targetRouteKind, targetRouteID, targetRouteVariant, targetExternal string
		var redirectMode, queryMode, replaceQuery string
		var changedAt sql.NullTime
		if err := refRows.Scan(
			&path, &renderedQuery, &kind,
			&ownerKind, &ownerID, &ownerVariant,
			&targetKind, &targetRouteKind, &targetRouteID, &targetRouteVariant,
			&targetExternal, &redirectMode, &queryMode, &replaceQuery, &changedAt,
		); err != nil {
			_ = refRows.Close()
			return registryState{}, unavailable("scan reference", err)
		}
		lookup, err := adapter.catalog.normalizeLookup(Lookup{EscapedPath: path, RawQuery: renderedQuery})
		if err != nil {
			_ = refRows.Close()
			return registryState{}, &Error{Kind: ErrorCorruptState, Field: "postgres.reference", Message: err.Error()}
		}
		ref := storedReference{
			Ref: lookup.ref, Kind: referenceKind(kind),
			Owner: RouteKey{Resource: ResourceKey{Kind: ResourceKind(ownerKind), ID: ownerID}, Variant: ownerVariant},
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
		if changedAt.Valid {
			ref.ChangedAt = changedAt.Time.UTC()
		}
		state.Refs[lookup.ref.key] = ref
	}
	if err := refRows.Close(); err != nil {
		return registryState{}, unavailable("close references", err)
	}
	if err := refRows.Err(); err != nil {
		return registryState{}, unavailable("iterate references", err)
	}

	routeRows, err := tx.QueryContext(ctx, `
SELECT resource_kind, resource_id, variant, canonical_ref_id, route_revision, changed_at
FROM `+adapter.table("routes")+`
WHERE instance_key = $1`, adapter.instanceKey)
	if err != nil {
		return registryState{}, unavailable("list routes", err)
	}
	for routeRows.Next() {
		var kind, id, variant, canonicalRefID string
		var revision RouteRevision
		var changedAt sql.NullTime
		if err := routeRows.Scan(&kind, &id, &variant, &canonicalRefID, &revision, &changedAt); err != nil {
			_ = routeRows.Close()
			return registryState{}, unavailable("scan route", err)
		}
		key := RouteKey{Resource: ResourceKey{Kind: ResourceKind(kind), ID: id}, Variant: variant}
		var canonical normalizedRef
		for _, ref := range state.Refs {
			if storageRefID(ref.Ref) == canonicalRefID {
				canonical = ref.Ref
				break
			}
		}
		if canonical.key == "" {
			_ = routeRows.Close()
			return registryState{}, &Error{Kind: ErrorCorruptState, Field: "postgres.route", Message: "canonical reference is missing"}
		}
		route := storedRoute{Key: key, Canonical: canonical, Aliases: map[string]storedAlias{}, Revision: revision}
		if changedAt.Valid {
			route.ChangedAt = changedAt.Time.UTC()
		}
		state.Routes[routeMapKey(key)] = route
	}
	if err := routeRows.Close(); err != nil {
		return registryState{}, unavailable("close routes", err)
	}
	if err := routeRows.Err(); err != nil {
		return registryState{}, unavailable("iterate routes", err)
	}
	for refKey, ref := range state.Refs {
		if ref.Kind != referenceAlias {
			continue
		}
		route, exists := state.Routes[routeMapKey(ref.Owner)]
		if !exists {
			return registryState{}, &Error{Kind: ErrorCorruptState, Field: "postgres.alias", Message: "alias owner is missing"}
		}
		route.Aliases[refKey] = storedAlias{Ref: ref.Ref, Policy: ref.Policy}
		state.Routes[routeMapKey(ref.Owner)] = route
	}

	overlayRows, err := tx.QueryContext(ctx, `
SELECT ref_id, owner_kind, owner_id, owner_variant,
       target_kind, target_route_kind, target_route_id, target_route_variant,
       target_external, redirect_mode, query_mode, replace_query, expires_at, changed_at
FROM `+adapter.table("overlays")+`
WHERE instance_key = $1`, adapter.instanceKey)
	if err != nil {
		return registryState{}, unavailable("list overlays", err)
	}
	for overlayRows.Next() {
		var refID, ownerKind, ownerID, ownerVariant string
		var targetKind, targetRouteKind, targetRouteID, targetRouteVariant, targetExternal string
		var redirectMode, queryMode, replaceQuery string
		var expiresAt, changedAt sql.NullTime
		if err := overlayRows.Scan(
			&refID, &ownerKind, &ownerID, &ownerVariant,
			&targetKind, &targetRouteKind, &targetRouteID, &targetRouteVariant,
			&targetExternal, &redirectMode, &queryMode, &replaceQuery, &expiresAt, &changedAt,
		); err != nil {
			_ = overlayRows.Close()
			return registryState{}, unavailable("scan overlay", err)
		}
		var source normalizedRef
		for _, ref := range state.Refs {
			if storageRefID(ref.Ref) == refID {
				source = ref.Ref
				break
			}
		}
		if source.key == "" {
			_ = overlayRows.Close()
			return registryState{}, &Error{Kind: ErrorCorruptState, Field: "postgres.overlay", Message: "base reference is missing"}
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
		overlay := storedOverlay{
			Owner: RouteKey{
				Resource: ResourceKey{Kind: ResourceKind(ownerKind), ID: ownerID},
				Variant:  ownerVariant,
			},
			Source: source, Redirect: redirect,
		}
		if changedAt.Valid {
			overlay.ChangedAt = changedAt.Time.UTC()
		}
		state.Overlays[source.key] = overlay
	}
	if err := overlayRows.Close(); err != nil {
		return registryState{}, unavailable("close overlays", err)
	}
	if err := overlayRows.Err(); err != nil {
		return registryState{}, unavailable("iterate overlays", err)
	}

	commandRows, err := tx.QueryContext(ctx, `
SELECT command_id, intent_digest, receipt
FROM `+adapter.table("commands")+`
WHERE instance_key = $1`, adapter.instanceKey)
	if err != nil {
		return registryState{}, unavailable("list commands", err)
	}
	for commandRows.Next() {
		var id string
		var digest string
		var encoded []byte
		if err := commandRows.Scan(&id, &digest, &encoded); err != nil {
			_ = commandRows.Close()
			return registryState{}, unavailable("scan command", err)
		}
		var receipt Receipt
		if err := json.Unmarshal(encoded, &receipt); err != nil {
			_ = commandRows.Close()
			return registryState{}, &Error{Kind: ErrorCorruptState, Field: "postgres.command", Message: "receipt is invalid", Cause: err}
		}
		state.Commands[CommandID(id)] = storedCommand{Digest: Digest(digest), Receipt: receipt}
	}
	if err := commandRows.Close(); err != nil {
		return registryState{}, unavailable("close commands", err)
	}
	if err := commandRows.Err(); err != nil {
		return registryState{}, unavailable("iterate commands", err)
	}

	historyRows, err := tx.QueryContext(ctx, `
SELECT command_id, revision, actor_kind, actor_id, reason, applied_at
FROM `+adapter.table("history")+`
WHERE instance_key = $1
ORDER BY revision`, adapter.instanceKey)
	if err != nil {
		return registryState{}, unavailable("list history", err)
	}
	for historyRows.Next() {
		var item TransitionSummary
		if err := historyRows.Scan(
			&item.CommandID, &item.Revision, &item.Actor.Kind, &item.Actor.ID, &item.Reason, &item.AppliedAt,
		); err != nil {
			_ = historyRows.Close()
			return registryState{}, unavailable("scan history", err)
		}
		item.AppliedAt = item.AppliedAt.UTC()
		state.History = append(state.History, item)
	}
	if err := historyRows.Close(); err != nil {
		return registryState{}, unavailable("close history", err)
	}
	if err := historyRows.Err(); err != nil {
		return registryState{}, unavailable("iterate history", err)
	}
	if err := validateFinalState(adapter.catalog, state); err != nil {
		return registryState{}, err
	}
	return state, nil
}

func (adapter *PostgresAdapter) persistState(
	ctx context.Context,
	tx *sql.Tx,
	before, after registryState,
) error {
	for key, value := range before.Overlays {
		if next, exists := after.Overlays[key]; !exists || !reflect.DeepEqual(value, next) {
			if _, err := tx.ExecContext(ctx, `
DELETE FROM `+adapter.table("overlays")+`
WHERE instance_key = $1 AND ref_id = $2`, adapter.instanceKey, storageRefID(value.Source)); err != nil {
				return unavailable("delete overlay", err)
			}
		}
	}
	for key, value := range before.Refs {
		if _, exists := after.Refs[key]; !exists {
			if _, err := tx.ExecContext(ctx, `
DELETE FROM `+adapter.table("references")+`
WHERE instance_key = $1 AND ref_id = $2`, adapter.instanceKey, storageRefID(value.Ref)); err != nil {
				return unavailable("delete reference", err)
			}
		}
	}
	for key, value := range before.Routes {
		if _, exists := after.Routes[key]; !exists {
			if _, err := tx.ExecContext(ctx, `
DELETE FROM `+adapter.table("routes")+`
WHERE instance_key = $1 AND route_key = $2`, adapter.instanceKey, storageRouteID(value.Key)); err != nil {
				return unavailable("delete route", err)
			}
		}
	}
	for key, value := range after.Refs {
		if previous, exists := before.Refs[key]; exists && reflect.DeepEqual(previous, value) {
			continue
		}
		if err := adapter.upsertReference(ctx, tx, value); err != nil {
			return err
		}
	}
	for key, value := range after.Routes {
		if previous, exists := before.Routes[key]; exists && reflect.DeepEqual(previous, value) {
			continue
		}
		if err := adapter.upsertRoute(ctx, tx, value); err != nil {
			return err
		}
	}
	for key, value := range after.Overlays {
		if previous, exists := before.Overlays[key]; exists && reflect.DeepEqual(previous, value) {
			continue
		}
		if err := adapter.upsertOverlay(ctx, tx, value); err != nil {
			return err
		}
	}
	for id, command := range after.Commands {
		if _, exists := before.Commands[id]; exists {
			continue
		}
		encoded, err := json.Marshal(command.Receipt)
		if err != nil {
			return unavailable("encode command receipt", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO `+adapter.table("commands")+` (
    instance_key, command_id, intent_digest, receipt
)
VALUES ($1, $2, $3, $4::jsonb)`,
			adapter.instanceKey, id, command.Digest, string(encoded),
		); err != nil {
			return unavailable("insert command", err)
		}
	}
	if len(after.History) > len(before.History) {
		for _, item := range after.History[len(before.History):] {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO `+adapter.table("history")+` (
    instance_key, revision, command_id, actor_kind, actor_id, reason, applied_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				adapter.instanceKey, item.Revision, item.CommandID,
				item.Actor.Kind, item.Actor.ID, item.Reason, item.AppliedAt,
			); err != nil {
				return unavailable("insert history", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE `+adapter.table("instances")+`
SET revision = $2, updated_at = $3
WHERE instance_key = $1`, adapter.instanceKey, after.Revision, adapter.clock().UTC()); err != nil {
		return unavailable("update instance revision", err)
	}
	return nil
}

func (adapter *PostgresAdapter) upsertReference(
	ctx context.Context,
	tx *sql.Tx,
	value storedReference,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO `+adapter.table("references")+` (
    instance_key, ref_id, namespace, path, identity_query, rendered_query, kind,
    owner_kind, owner_id, owner_variant,
    target_kind, target_route_kind, target_route_id, target_route_variant,
    target_external, redirect_mode, query_mode, replace_query, changed_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
)
ON CONFLICT (instance_key, ref_id) DO UPDATE SET
    namespace = EXCLUDED.namespace,
    path = EXCLUDED.path,
    identity_query = EXCLUDED.identity_query,
    rendered_query = EXCLUDED.rendered_query,
    kind = EXCLUDED.kind,
    owner_kind = EXCLUDED.owner_kind,
    owner_id = EXCLUDED.owner_id,
    owner_variant = EXCLUDED.owner_variant,
    target_kind = EXCLUDED.target_kind,
    target_route_kind = EXCLUDED.target_route_kind,
    target_route_id = EXCLUDED.target_route_id,
    target_route_variant = EXCLUDED.target_route_variant,
    target_external = EXCLUDED.target_external,
    redirect_mode = EXCLUDED.redirect_mode,
    query_mode = EXCLUDED.query_mode,
    replace_query = EXCLUDED.replace_query,
    changed_at = EXCLUDED.changed_at`,
		adapter.instanceKey, storageRefID(value.Ref), value.Ref.namespace,
		value.Ref.ref.Path, value.Ref.identity, value.Ref.query, value.Kind,
		value.Owner.Resource.Kind, value.Owner.Resource.ID, value.Owner.Variant,
		value.Target.Kind, value.Target.Route.Resource.Kind, value.Target.Route.Resource.ID,
		value.Target.Route.Variant, value.Target.External,
		value.Policy.Mode, value.Policy.Query, value.Policy.ReplaceQuery, value.ChangedAt,
	)
	if err != nil {
		return unavailable("upsert reference", err)
	}
	return nil
}

func (adapter *PostgresAdapter) upsertRoute(
	ctx context.Context,
	tx *sql.Tx,
	value storedRoute,
) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO `+adapter.table("routes")+` (
    instance_key, route_key, resource_kind, resource_id, variant,
    canonical_ref_id, route_revision, changed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (instance_key, route_key) DO UPDATE SET
    resource_kind = EXCLUDED.resource_kind,
    resource_id = EXCLUDED.resource_id,
    variant = EXCLUDED.variant,
    canonical_ref_id = EXCLUDED.canonical_ref_id,
    route_revision = EXCLUDED.route_revision,
    changed_at = EXCLUDED.changed_at`,
		adapter.instanceKey, storageRouteID(value.Key), value.Key.Resource.Kind,
		value.Key.Resource.ID, value.Key.Variant, storageRefID(value.Canonical),
		value.Revision, value.ChangedAt,
	)
	if err != nil {
		return unavailable("upsert route", err)
	}
	return nil
}

func (adapter *PostgresAdapter) upsertOverlay(
	ctx context.Context,
	tx *sql.Tx,
	value storedOverlay,
) error {
	var expiresAt any
	if value.Redirect.ExpiresAt != nil {
		expiresAt = value.Redirect.ExpiresAt.UTC()
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO `+adapter.table("overlays")+` (
    instance_key, ref_id, owner_kind, owner_id, owner_variant,
    target_kind, target_route_kind, target_route_id, target_route_variant,
    target_external, redirect_mode, query_mode, replace_query, expires_at, changed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (instance_key, ref_id) DO UPDATE SET
    owner_kind = EXCLUDED.owner_kind,
    owner_id = EXCLUDED.owner_id,
    owner_variant = EXCLUDED.owner_variant,
    target_kind = EXCLUDED.target_kind,
    target_route_kind = EXCLUDED.target_route_kind,
    target_route_id = EXCLUDED.target_route_id,
    target_route_variant = EXCLUDED.target_route_variant,
    target_external = EXCLUDED.target_external,
    redirect_mode = EXCLUDED.redirect_mode,
    query_mode = EXCLUDED.query_mode,
    replace_query = EXCLUDED.replace_query,
    expires_at = EXCLUDED.expires_at,
    changed_at = EXCLUDED.changed_at`,
		adapter.instanceKey, storageRefID(value.Source),
		value.Owner.Resource.Kind, value.Owner.Resource.ID, value.Owner.Variant,
		value.Redirect.Target.Kind, value.Redirect.Target.Route.Resource.Kind,
		value.Redirect.Target.Route.Resource.ID, value.Redirect.Target.Route.Variant,
		value.Redirect.Target.External, value.Redirect.Policy.Mode,
		value.Redirect.Policy.Query, value.Redirect.Policy.ReplaceQuery,
		expiresAt, value.ChangedAt,
	)
	if err != nil {
		return unavailable("upsert overlay", err)
	}
	return nil
}
