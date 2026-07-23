package urllifecycle

import (
	"slices"
	"strings"
	"time"
)

func inspectState(
	catalog *Catalog,
	state registryState,
	query InspectQuery,
	now time.Time,
) (Inspection, error) {
	switch {
	case query.Route != nil && query.Ref != nil:
		return Inspection{}, invalid("inspect", "accepts route or ref, not both")
	case query.Route != nil:
		if err := catalog.validateRouteKey(*query.Route); err != nil {
			return Inspection{}, invalid("route", "%v", err)
		}
		route, exists := state.Routes[routeMapKey(*query.Route)]
		if !exists {
			return Inspection{}, &Error{Kind: ErrorNotFound, Field: "route", Message: "route is not active"}
		}
		active := activeRouteValue(route)
		lookup := normalizedLookup{ref: route.Canonical}
		resolution, err := resolveState(catalog, state, lookup, now)
		if err != nil {
			return Inspection{}, err
		}
		result := Inspection{
			Route: query.Route, Active: &active, Resolution: resolution, Revision: route.Revision,
		}
		if overlay, exists := state.Overlays[route.Canonical.key]; exists {
			value := overlay.Redirect
			result.Overlay = &value
		}
		return result, nil
	case query.Ref != nil:
		ref, err := catalog.normalizeLocalRef(*query.Ref)
		if err != nil {
			return Inspection{}, err
		}
		resolution, err := resolveState(catalog, state, normalizedLookup{ref: ref}, now)
		if err != nil {
			return Inspection{}, err
		}
		result := Inspection{Resolution: resolution}
		if stored, exists := state.Refs[ref.key]; exists && stored.Owner != (RouteKey{}) {
			routeKey := stored.Owner
			result.Route = &routeKey
			if route, active := state.Routes[routeMapKey(routeKey)]; active {
				value := activeRouteValue(route)
				result.Active = &value
				result.Revision = route.Revision
			}
		}
		if overlay, exists := state.Overlays[ref.key]; exists {
			value := overlay.Redirect
			result.Overlay = &value
		}
		return result, nil
	default:
		return Inspection{}, invalid("inspect", "requires route or ref")
	}
}

func activeRouteValue(route storedRoute) ActiveRoute {
	active := ActiveRoute{Canonical: route.Canonical.ref, Aliases: make([]Alias, 0, len(route.Aliases))}
	for _, alias := range route.Aliases {
		active.Aliases = append(active.Aliases, Alias{Ref: alias.Ref.ref, Policy: alias.Policy})
	}
	slices.SortFunc(active.Aliases, func(left, right Alias) int {
		if compared := strings.Compare(left.Ref.Path, right.Ref.Path); compared != 0 {
			return compared
		}
		return strings.Compare(encodeQuery(left.Ref.Query), encodeQuery(right.Ref.Query))
	})
	return active
}

func listState(
	catalog *Catalog,
	state registryState,
	query ListQuery,
	now time.Time,
) (InspectionPage, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > catalog.limits.MaxPageSize {
		return InspectionPage{}, &Error{Kind: ErrorLimitExceeded, Field: "limit", Message: "page size exceeds policy"}
	}
	keys := make([]string, 0, len(state.Refs))
	for key, ref := range state.Refs {
		if query.Prefix != "" && !strings.HasPrefix(ref.Ref.ref.Path, query.Prefix) {
			continue
		}
		if query.After != "" && key <= query.After {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	page := InspectionPage{Items: make([]Inspection, 0, min(limit, len(keys)))}
	selected := keys
	if len(selected) > limit {
		selected = selected[:limit]
		page.Next = selected[len(selected)-1]
	}
	for _, key := range selected {
		ref := state.Refs[key]
		value := ref.Ref.ref
		inspection, err := inspectState(catalog, state, InspectQuery{Ref: &value}, now)
		if err != nil {
			return InspectionPage{}, err
		}
		page.Items = append(page.Items, inspection)
	}
	return page, nil
}

func historyState(state registryState, query HistoryQuery, maxPageSize int) HistoryPage {
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	matchingRevisions := map[Revision]struct{}{}
	if query.Route != nil {
		for _, command := range state.Commands {
			for _, route := range command.Receipt.RouteRevisions {
				if route.Route == *query.Route {
					matchingRevisions[command.Receipt.Revision] = struct{}{}
					break
				}
			}
		}
	}
	page := HistoryPage{Items: []TransitionSummary{}}
	for _, item := range state.History {
		if item.Revision <= query.AfterRevision {
			continue
		}
		if query.Route != nil {
			if _, matches := matchingRevisions[item.Revision]; !matches {
				continue
			}
		}
		if len(page.Items) == limit {
			page.Next = page.Items[len(page.Items)-1].Revision
			break
		}
		page.Items = append(page.Items, item)
	}
	return page
}
