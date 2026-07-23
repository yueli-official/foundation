package urllifecycle

func DefaultPermanentRedirect() RedirectPolicy {
	return RedirectPolicy{
		Mode:  PermanentPreserveMethod,
		Query: QueryCanonicalWithExtras,
	}
}

func DefaultTemporaryRedirect() RedirectPolicy {
	return RedirectPolicy{
		Mode:  TemporaryPreserveMethod,
		Query: QueryCanonicalWithExtras,
	}
}

type ClaimSpec struct {
	Route  RouteKey
	Active ActiveRoute
}

func Claim(meta MutationMeta, claims ...ClaimSpec) ChangeSet {
	changes := make([]ResourceChange, 0, len(claims))
	for _, claim := range claims {
		active := claim.Active
		changes = append(changes, ResourceChange{
			Route: claim.Route, Desired: &active,
			Departures: DeparturePolicy{
				Canonical: FormerOutcome{Kind: FormerRelease},
				Aliases:   FormerOutcome{Kind: FormerRelease},
			},
		})
	}
	return changeSet(meta, changes, nil)
}

func Rename(
	meta MutationMeta,
	route RouteKey,
	expected RouteRevision,
	current ActiveRoute,
	next LocalRef,
	policy RedirectPolicy,
) ChangeSet {
	current.Canonical = next
	return changeSet(meta, []ResourceChange{{
		Route: route, ExpectedRevision: expected, Desired: &current,
		Departures: DeparturePolicy{
			Canonical: FormerOutcome{Kind: FormerRedirectToCurrent, Redirect: policy},
			Aliases:   FormerOutcome{Kind: FormerRelease},
		},
	}}, nil)
}

func Merge(
	meta MutationMeta,
	source RouteKey,
	expected RouteRevision,
	target RouteKey,
	policy RedirectPolicy,
) ChangeSet {
	return changeSet(meta, []ResourceChange{{
		Route: source, ExpectedRevision: expected,
		Departures: DeparturePolicy{
			Canonical: FormerOutcome{Kind: FormerRedirectToRoute, Target: target, Redirect: policy},
			Aliases:   FormerOutcome{Kind: FormerRedirectToRoute, Target: target, Redirect: policy},
		},
	}}, nil)
}

type RouteMove struct {
	Route            RouteKey
	ExpectedRevision RouteRevision
	Current          ActiveRoute
	To               LocalRef
}

func Rebase(meta MutationMeta, moves []RouteMove, policy RedirectPolicy) ChangeSet {
	changes := make([]ResourceChange, 0, len(moves))
	for _, move := range moves {
		next := move.Current
		next.Canonical = move.To
		changes = append(changes, ResourceChange{
			Route: move.Route, ExpectedRevision: move.ExpectedRevision, Desired: &next,
			Departures: DeparturePolicy{
				Canonical: FormerOutcome{Kind: FormerRedirectToCurrent, Redirect: policy},
				Aliases:   FormerOutcome{Kind: FormerRelease},
			},
		})
	}
	return changeSet(meta, changes, nil)
}

type RetireItem struct {
	Route            RouteKey
	ExpectedRevision RouteRevision
	Canonical        FormerOutcome
	Aliases          FormerOutcome
}

func Retire(meta MutationMeta, items ...RetireItem) ChangeSet {
	changes := make([]ResourceChange, 0, len(items))
	for _, item := range items {
		changes = append(changes, ResourceChange{
			Route: item.Route, ExpectedRevision: item.ExpectedRevision,
			Departures: DeparturePolicy{Canonical: item.Canonical, Aliases: item.Aliases},
		})
	}
	return changeSet(meta, changes, nil)
}

func RetireGone(route RouteKey, expected RouteRevision) RetireItem {
	return RetireItem{
		Route: route, ExpectedRevision: expected,
		Canonical: FormerOutcome{Kind: FormerGone},
		Aliases:   FormerOutcome{Kind: FormerGone},
	}
}

func ReleaseRoute(route RouteKey, expected RouteRevision) RetireItem {
	return RetireItem{
		Route: route, ExpectedRevision: expected,
		Canonical: FormerOutcome{Kind: FormerRelease},
		Aliases:   FormerOutcome{Kind: FormerRelease},
	}
}

func SetTemporaryRedirect(meta MutationMeta, change OverlayChange) ChangeSet {
	return changeSet(meta, nil, []OverlayChange{change})
}

func changeSet(meta MutationMeta, resources []ResourceChange, overlays []OverlayChange) ChangeSet {
	return ChangeSet{
		CommandID: meta.CommandID, Actor: meta.Actor, Reason: meta.Reason,
		ExpectedHead: meta.ExpectedHead, ResourceChanges: resources, OverlayChanges: overlays,
	}
}
