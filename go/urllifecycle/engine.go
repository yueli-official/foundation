package urllifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

type normalizedResourceChange struct {
	source  ResourceChange
	desired *storedRoute
	newKeys map[string]struct{}
}

type transitionResult struct {
	plan    Plan
	next    registryState
	receipt Receipt
	replay  *Receipt
}

func planTransition(
	catalog *Catalog,
	state registryState,
	changeSet ChangeSet,
	now time.Time,
) (transitionResult, error) {
	if catalog == nil {
		return transitionResult{}, invalid("catalog", "is required")
	}
	if strings.TrimSpace(string(changeSet.CommandID)) == "" {
		return transitionResult{}, invalid("command_id", "is required")
	}
	if strings.TrimSpace(changeSet.Reason) == "" {
		return transitionResult{}, invalid("reason", "is required")
	}
	if len(changeSet.Reason) > catalog.limits.MaxReasonBytes {
		return transitionResult{}, invalid("reason", "exceeds %d bytes", catalog.limits.MaxReasonBytes)
	}
	changeCount := len(changeSet.ResourceChanges) + len(changeSet.OverlayChanges)
	if changeCount == 0 {
		return transitionResult{}, invalid("changes", "must not be empty")
	}
	if changeCount > catalog.limits.MaxChanges {
		return transitionResult{}, &Error{
			Kind: ErrorLimitExceeded, Field: "changes",
			Message: fmt.Sprintf("exceeds %d", catalog.limits.MaxChanges),
		}
	}

	normalized, canonicalForDigest, err := normalizeChanges(catalog, changeSet, state)
	if err != nil {
		return transitionResult{}, err
	}
	digest, err := intentDigest(canonicalForDigest)
	if err != nil {
		return transitionResult{}, err
	}
	basePlan := Plan{Valid: true, BaseRevision: state.Revision, IntentDigest: digest, Effects: []Effect{}}
	if stored, exists := state.Commands[changeSet.CommandID]; exists {
		if stored.Digest != digest {
			return transitionResult{}, &Error{
				Kind: ErrorIdempotencyConflict, Field: "command_id",
				Message: "is reused with a different normalized intent",
			}
		}
		receipt := stored.Receipt
		return transitionResult{plan: basePlan, next: state, receipt: receipt, replay: &receipt}, nil
	}
	if changeSet.ExpectedHead != 0 && changeSet.ExpectedHead != state.Revision {
		return transitionResult{}, &Error{
			Kind: ErrorStaleRevision, Field: "expected_head",
			Message: fmt.Sprintf("expected %d, current is %d", changeSet.ExpectedHead, state.Revision),
		}
	}

	next := state.clone()
	effects := make([]Effect, 0, changeCount*2)
	removed := make(map[string]storedReference)
	changedRoutes := make(map[string]RouteKey)

	// Remove every changed route first. The final-state planner can therefore
	// validate batches without depending on caller-provided ordering.
	for _, change := range normalized.resources {
		key := routeMapKey(change.source.Route)
		current, exists := state.Routes[key]
		if change.source.ExpectedRevision != 0 {
			if !exists || current.Revision != change.source.ExpectedRevision {
				return transitionResult{}, &Error{
					Kind: ErrorStaleRevision, Field: "resource_changes.expected_revision",
					Message: fmt.Sprintf("route %q revision no longer matches", key),
				}
			}
		}
		if exists {
			if ref, present := next.Refs[current.Canonical.key]; present {
				removed[current.Canonical.key] = ref
				delete(next.Refs, current.Canonical.key)
				delete(next.Overlays, current.Canonical.key)
			}
			for aliasKey := range current.Aliases {
				if ref, present := next.Refs[aliasKey]; present {
					removed[aliasKey] = ref
					delete(next.Refs, aliasKey)
					delete(next.Overlays, aliasKey)
				}
			}
		}
		delete(next.Routes, key)
		changedRoutes[key] = change.source.Route
	}

	// Install the desired canonical and alias claims.
	for index := range normalized.resources {
		change := &normalized.resources[index]
		if change.desired == nil {
			continue
		}
		key := routeMapKey(change.source.Route)
		current, existed := state.Routes[key]
		routeRevision := RouteRevision(1)
		if existed {
			routeRevision = current.Revision + 1
		}
		change.desired.Revision = routeRevision
		change.desired.ChangedAt = now
		if err := putReference(next.Refs, storedReference{
			Ref: change.desired.Canonical, Kind: referenceCanonical,
			Owner: change.source.Route, ChangedAt: now,
		}); err != nil {
			return transitionResult{}, err
		}
		routeValue := change.source.Route
		effects = append(effects, Effect{Kind: EffectClaim, Ref: change.desired.Canonical.ref, Route: &routeValue})
		for _, alias := range change.desired.Aliases {
			if err := putReference(next.Refs, storedReference{
				Ref: alias.Ref, Kind: referenceAlias, Owner: change.source.Route,
				Policy: alias.Policy, ChangedAt: now,
			}); err != nil {
				return transitionResult{}, err
			}
			aliasRoute := change.source.Route
			effects = append(effects, Effect{Kind: EffectAlias, Ref: alias.Ref.ref, Route: &aliasRoute})
		}
		next.Routes[key] = *change.desired
	}

	// Former references are explicit. A released former URL leaves no active
	// row; every other outcome must still be free after the desired state lands.
	for _, change := range normalized.resources {
		current, existed := state.Routes[routeMapKey(change.source.Route)]
		if !existed {
			continue
		}
		if _, retained := change.newKeys[current.Canonical.key]; !retained {
			ref := removed[current.Canonical.key]
			effect, err := applyFormer(catalog, &next, ref, change.source.Route, change.source.Departures.Canonical, now)
			if err != nil {
				return transitionResult{}, err
			}
			if effect != nil {
				effects = append(effects, *effect)
			}
		}
		for aliasKey := range current.Aliases {
			if _, retained := change.newKeys[aliasKey]; retained {
				continue
			}
			ref := removed[aliasKey]
			effect, err := applyFormer(catalog, &next, ref, change.source.Route, change.source.Departures.Aliases, now)
			if err != nil {
				return transitionResult{}, err
			}
			if effect != nil {
				effects = append(effects, *effect)
			}
		}
	}

	// A retired Route cannot leave historical redirects dangling. Its inbound
	// former references inherit the explicit canonical departure outcome:
	// gone stays gone, release becomes reusable, and merge retargets directly to
	// the replacement Route.
	for _, change := range normalized.resources {
		if change.desired != nil {
			continue
		}
		inboundKeys := make([]string, 0)
		for key, ref := range next.Refs {
			if ref.Kind == referenceRedirect &&
				ref.Target.Kind == TargetRoute &&
				ref.Target.Route == change.source.Route {
				inboundKeys = append(inboundKeys, key)
			}
		}
		slices.Sort(inboundKeys)
		for _, key := range inboundKeys {
			ref := next.Refs[key]
			delete(next.Refs, key)
			delete(next.Overlays, key)
			effect, err := applyFormer(
				catalog, &next, ref, change.source.Route,
				change.source.Departures.Canonical, now,
			)
			if err != nil {
				return transitionResult{}, err
			}
			if effect != nil {
				effects = append(effects, *effect)
			}
		}
	}

	// Apply temporary overlays after the final base graph exists.
	for _, change := range normalized.overlays {
		base, exists := next.Refs[change.source.key]
		if !exists || (base.Kind != referenceCanonical && base.Kind != referenceAlias) || base.Owner != change.change.Owner {
			return transitionResult{}, conflict("overlay.source", "must be owned by the overlay route")
		}
		routeKey := routeMapKey(change.change.Owner)
		route, exists := next.Routes[routeKey]
		if !exists {
			return transitionResult{}, &Error{Kind: ErrorDanglingTarget, Field: "overlay.owner", Message: "route has no final canonical"}
		}
		if change.change.ExpectedRevision != 0 {
			original, existed := state.Routes[routeKey]
			if !existed || original.Revision != change.change.ExpectedRevision {
				return transitionResult{}, &Error{Kind: ErrorStaleRevision, Field: "overlay.expected_revision", Message: "route revision no longer matches"}
			}
		}
		if change.change.Desired == nil {
			if _, exists := next.Overlays[change.source.key]; exists {
				delete(next.Overlays, change.source.key)
				effects = append(effects, Effect{Kind: EffectOverlayDrop, Ref: change.source.ref})
			}
		} else {
			desired := *change.change.Desired
			if err := validateTarget(catalog, next, desired.Target); err != nil {
				return transitionResult{}, err
			}
			if desired.Target.Kind == TargetRoute {
				targetRoute := next.Routes[routeMapKey(desired.Target.Route)]
				if targetRoute.Canonical.key == change.source.key {
					return transitionResult{}, &Error{
						Kind: ErrorCycle, Field: "overlay.target",
						Message: "temporary redirect would target its own source",
					}
				}
			}
			next.Overlays[change.source.key] = storedOverlay{
				Owner: change.change.Owner, Source: change.source, Redirect: desired, ChangedAt: now,
			}
			target := desired.Target
			effects = append(effects, Effect{Kind: EffectOverlaySet, Ref: change.source.ref, Target: &target})
		}
		if _, alreadyChanged := changedRoutes[routeKey]; !alreadyChanged {
			route.Revision++
			route.ChangedAt = now
			next.Routes[routeKey] = route
			changedRoutes[routeKey] = change.change.Owner
		}
	}

	if err := validateFinalState(catalog, next); err != nil {
		return transitionResult{}, err
	}
	sortEffects(catalog, effects)
	next.Revision = state.Revision + 1
	routeRevisions := make([]RouteRevisionResult, 0, len(changedRoutes))
	for _, route := range changedRoutes {
		if stored, exists := next.Routes[routeMapKey(route)]; exists {
			routeRevisions = append(routeRevisions, RouteRevisionResult{Route: route, Revision: stored.Revision})
		} else if old, exists := state.Routes[routeMapKey(route)]; exists {
			routeRevisions = append(routeRevisions, RouteRevisionResult{Route: route, Revision: old.Revision + 1})
		}
	}
	slices.SortFunc(routeRevisions, func(left, right RouteRevisionResult) int {
		return strings.Compare(routeMapKey(left.Route), routeMapKey(right.Route))
	})
	receipt := Receipt{
		CommandID: changeSet.CommandID, IntentDigest: digest, Revision: next.Revision,
		RouteRevisions: routeRevisions, Effects: effects,
	}
	next.Commands[changeSet.CommandID] = storedCommand{Digest: digest, Receipt: receipt}
	next.History = append(next.History, TransitionSummary{
		CommandID: changeSet.CommandID, Revision: next.Revision, Actor: changeSet.Actor,
		Reason: changeSet.Reason, AppliedAt: now,
	})
	basePlan.Effects = effects
	return transitionResult{plan: basePlan, next: next, receipt: receipt}, nil
}

type normalizedChanges struct {
	resources []normalizedResourceChange
	overlays  []normalizedOverlayChange
}

type normalizedOverlayChange struct {
	change OverlayChange
	source normalizedRef
}

func normalizeChanges(
	catalog *Catalog,
	changeSet ChangeSet,
	state registryState,
) (normalizedChanges, any, error) {
	result := normalizedChanges{
		resources: make([]normalizedResourceChange, 0, len(changeSet.ResourceChanges)),
		overlays:  make([]normalizedOverlayChange, 0, len(changeSet.OverlayChanges)),
	}
	seenRoutes := map[string]struct{}{}
	for index, change := range changeSet.ResourceChanges {
		if err := catalog.validateRouteKey(change.Route); err != nil {
			return normalizedChanges{}, nil, invalid(fmt.Sprintf("resource_changes[%d].route", index), "%v", err)
		}
		key := routeMapKey(change.Route)
		if _, exists := seenRoutes[key]; exists {
			return normalizedChanges{}, nil, invalid("resource_changes", "contains duplicate route")
		}
		seenRoutes[key] = struct{}{}
		normalized := normalizedResourceChange{source: change, newKeys: map[string]struct{}{}}
		if change.Desired != nil {
			canonical, err := catalog.normalizeLocalRef(change.Desired.Canonical)
			if err != nil {
				return normalizedChanges{}, nil, invalid(fmt.Sprintf("resource_changes[%d].desired.canonical", index), "%v", err)
			}
			aliases := make(map[string]storedAlias, len(change.Desired.Aliases))
			if len(change.Desired.Aliases) > catalog.limits.MaxAliasesPerRoute {
				return normalizedChanges{}, nil, &Error{Kind: ErrorLimitExceeded, Field: "aliases", Message: "too many aliases"}
			}
			normalized.newKeys[canonical.key] = struct{}{}
			for aliasIndex, alias := range change.Desired.Aliases {
				ref, err := catalog.normalizeLocalRef(alias.Ref)
				if err != nil {
					return normalizedChanges{}, nil, invalid(fmt.Sprintf("resource_changes[%d].aliases[%d]", index, aliasIndex), "%v", err)
				}
				if _, exists := normalized.newKeys[ref.key]; exists {
					return normalizedChanges{}, nil, invalid("aliases", "contains the canonical or a duplicate alias")
				}
				policy, err := normalizeRedirectPolicy(alias.Policy, true)
				if err != nil {
					return normalizedChanges{}, nil, invalid("aliases.policy", "%v", err)
				}
				normalized.newKeys[ref.key] = struct{}{}
				aliases[ref.key] = storedAlias{Ref: ref, Policy: policy}
			}
			normalized.desired = &storedRoute{Key: change.Route, Canonical: canonical, Aliases: aliases}
		}
		if err := normalizeDeparturePolicy(&normalized.source.Departures); err != nil {
			return normalizedChanges{}, nil, invalid(fmt.Sprintf("resource_changes[%d].departures", index), "%v", err)
		}
		result.resources = append(result.resources, normalized)
	}
	seenOverlays := map[string]struct{}{}
	for index, change := range changeSet.OverlayChanges {
		if err := catalog.validateRouteKey(change.Owner); err != nil {
			return normalizedChanges{}, nil, invalid(fmt.Sprintf("overlay_changes[%d].owner", index), "%v", err)
		}
		source, err := catalog.normalizeLocalRef(change.Source)
		if err != nil {
			return normalizedChanges{}, nil, invalid(fmt.Sprintf("overlay_changes[%d].source", index), "%v", err)
		}
		if _, exists := seenOverlays[source.key]; exists {
			return normalizedChanges{}, nil, invalid("overlay_changes", "contains duplicate source")
		}
		seenOverlays[source.key] = struct{}{}
		if change.Desired != nil {
			policy, err := normalizeRedirectPolicy(change.Desired.Policy, false)
			if err != nil {
				return normalizedChanges{}, nil, invalid(fmt.Sprintf("overlay_changes[%d].policy", index), "%v", err)
			}
			change.Desired.Policy = policy
			if change.Desired.ExpiresAt != nil {
				value := change.Desired.ExpiresAt.UTC()
				change.Desired.ExpiresAt = &value
			}
		}
		result.overlays = append(result.overlays, normalizedOverlayChange{change: change, source: source})
	}

	canonical := canonicalIntent{
		CommandID: changeSet.CommandID, Actor: changeSet.Actor, Reason: strings.TrimSpace(changeSet.Reason),
		ExpectedHead: changeSet.ExpectedHead,
	}
	for _, change := range result.resources {
		item := canonicalResourceChange{
			Route: change.source.Route, ExpectedRevision: change.source.ExpectedRevision,
			Departures: change.source.Departures,
		}
		if change.desired != nil {
			item.Canonical = change.desired.Canonical.key
			for _, alias := range change.desired.Aliases {
				item.Aliases = append(item.Aliases, canonicalAlias{Ref: alias.Ref.key, Policy: alias.Policy})
			}
			slices.SortFunc(item.Aliases, func(left, right canonicalAlias) int { return strings.Compare(left.Ref, right.Ref) })
		}
		canonical.Resources = append(canonical.Resources, item)
	}
	slices.SortFunc(canonical.Resources, func(left, right canonicalResourceChange) int {
		return strings.Compare(routeMapKey(left.Route), routeMapKey(right.Route))
	})
	for _, overlay := range result.overlays {
		item := canonicalOverlayChange{
			Owner: overlay.change.Owner, Source: overlay.source.key,
			ExpectedRevision: overlay.change.ExpectedRevision, Desired: overlay.change.Desired,
		}
		canonical.Overlays = append(canonical.Overlays, item)
	}
	slices.SortFunc(canonical.Overlays, func(left, right canonicalOverlayChange) int {
		return strings.Compare(left.Source, right.Source)
	})
	return result, canonical, nil
}

type canonicalIntent struct {
	CommandID    CommandID
	Actor        ActorRef
	Reason       string
	ExpectedHead Revision
	Resources    []canonicalResourceChange
	Overlays     []canonicalOverlayChange
}

type canonicalResourceChange struct {
	Route            RouteKey
	ExpectedRevision RouteRevision
	Canonical        string
	Aliases          []canonicalAlias
	Departures       DeparturePolicy
}

type canonicalAlias struct {
	Ref    string
	Policy RedirectPolicy
}

type canonicalOverlayChange struct {
	Owner            RouteKey
	Source           string
	ExpectedRevision RouteRevision
	Desired          *TemporaryRedirect
}

func intentDigest(value any) (Digest, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", &Error{Kind: ErrorUnavailable, Field: "intent", Message: "cannot encode normalized intent", Cause: err}
	}
	sum := sha256.Sum256(encoded)
	return Digest(hex.EncodeToString(sum[:])), nil
}

func (catalog *Catalog) validateRouteKey(value RouteKey) error {
	if _, exists := catalog.resourceKinds[value.Resource.Kind]; !exists {
		return fmt.Errorf("resource kind %q is not registered", value.Resource.Kind)
	}
	if strings.TrimSpace(value.Resource.ID) == "" || len(value.Resource.ID) > catalog.limits.MaxResourceIDBytes ||
		strings.ContainsRune(value.Resource.ID, '\x00') {
		return fmt.Errorf("resource id is invalid")
	}
	if len(value.Variant) > catalog.limits.MaxVariantBytes || strings.ContainsRune(value.Variant, '\x00') {
		return fmt.Errorf("variant is invalid")
	}
	return nil
}

func normalizeDeparturePolicy(policy *DeparturePolicy) error {
	var err error
	policy.Canonical, err = normalizeFormer(policy.Canonical)
	if err != nil {
		return err
	}
	policy.Aliases, err = normalizeFormer(policy.Aliases)
	return err
}

func normalizeFormer(value FormerOutcome) (FormerOutcome, error) {
	if value.Kind == "" {
		value.Kind = FormerRelease
	}
	switch value.Kind {
	case FormerRelease, FormerGone:
		if value.Redirect.Mode != "" || value.Redirect.Query != "" || value.Target != (RouteKey{}) {
			return FormerOutcome{}, fmt.Errorf("%s must not include a target or redirect policy", value.Kind)
		}
	case FormerRedirectToCurrent:
		if value.Target != (RouteKey{}) {
			return FormerOutcome{}, fmt.Errorf("redirect_to_current must not include a target")
		}
		policy, err := normalizeRedirectPolicy(value.Redirect, true)
		if err != nil {
			return FormerOutcome{}, err
		}
		value.Redirect = policy
	case FormerRedirectToRoute:
		if value.Target == (RouteKey{}) {
			return FormerOutcome{}, fmt.Errorf("redirect_to_route requires a target")
		}
		policy, err := normalizeRedirectPolicy(value.Redirect, true)
		if err != nil {
			return FormerOutcome{}, err
		}
		value.Redirect = policy
	default:
		return FormerOutcome{}, fmt.Errorf("unknown former outcome %q", value.Kind)
	}
	return value, nil
}

func normalizeRedirectPolicy(value RedirectPolicy, permanent bool) (RedirectPolicy, error) {
	if value.Mode == "" {
		if permanent {
			value.Mode = PermanentPreserveMethod
		} else {
			value.Mode = TemporaryPreserveMethod
		}
	}
	if value.Query == "" {
		value.Query = QueryCanonicalWithExtras
	}
	if value.Mode.StatusCode() == 0 {
		return RedirectPolicy{}, fmt.Errorf("redirect mode is invalid")
	}
	if value.Mode.Permanent() != permanent {
		if permanent {
			return RedirectPolicy{}, fmt.Errorf("requires a permanent 301/308 mode")
		}
		return RedirectPolicy{}, fmt.Errorf("requires a temporary 302/307 mode")
	}
	switch value.Query {
	case QueryCanonicalWithExtras, QueryPreserve, QueryDrop:
		if value.ReplaceQuery != "" {
			return RedirectPolicy{}, fmt.Errorf("replace query is only valid with replace mode")
		}
	case QueryReplace:
		if strings.ContainsAny(value.ReplaceQuery, "#\r\n") {
			return RedirectPolicy{}, fmt.Errorf("replace query is invalid")
		}
	default:
		return RedirectPolicy{}, fmt.Errorf("query mode is invalid")
	}
	return value, nil
}

func putReference(values map[string]storedReference, value storedReference) error {
	if existing, exists := values[value.Ref.key]; exists {
		return conflict("local_ref", "%s is already %s for route %q", value.Ref.ref.Path, existing.Kind, routeMapKey(existing.Owner))
	}
	values[value.Ref.key] = value
	return nil
}

func applyFormer(
	catalog *Catalog,
	state *registryState,
	ref storedReference,
	owner RouteKey,
	outcome FormerOutcome,
	now time.Time,
) (*Effect, error) {
	if ref.Ref.key == "" {
		return nil, &Error{Kind: ErrorCorruptState, Field: "former_ref", Message: "missing former reference"}
	}
	if _, occupied := state.Refs[ref.Ref.key]; occupied {
		if outcome.Kind == FormerRelease {
			return nil, nil
		}
		return nil, conflict("former_ref", "%s is occupied by final state", ref.Ref.ref.Path)
	}
	switch outcome.Kind {
	case FormerRelease:
		return &Effect{Kind: EffectRelease, Ref: ref.Ref.ref}, nil
	case FormerGone:
		state.Refs[ref.Ref.key] = storedReference{
			Ref: ref.Ref, Kind: referenceGone, Owner: owner, ChangedAt: now,
		}
		route := owner
		return &Effect{Kind: EffectGone, Ref: ref.Ref.ref, Route: &route}, nil
	case FormerRedirectToCurrent, FormerRedirectToRoute:
		targetRoute := owner
		if outcome.Kind == FormerRedirectToRoute {
			targetRoute = outcome.Target
		}
		target := RouteTarget(targetRoute)
		if err := validateTarget(catalog, *state, target); err != nil {
			return nil, err
		}
		if terminal := state.Routes[routeMapKey(targetRoute)]; terminal.Canonical.key == ref.Ref.key {
			return nil, &Error{
				Kind: ErrorCycle, Field: "former_outcome",
				Message: "redirect would target its own source",
			}
		}
		state.Refs[ref.Ref.key] = storedReference{
			Ref: ref.Ref, Kind: referenceRedirect, Owner: owner,
			Target: target, Policy: outcome.Redirect, ChangedAt: now,
		}
		return &Effect{Kind: EffectRedirect, Ref: ref.Ref.ref, Target: &target}, nil
	default:
		return nil, invalid("former_outcome", "is invalid")
	}
}

func validateFinalState(catalog *Catalog, state registryState) error {
	for key, ref := range state.Refs {
		switch ref.Kind {
		case referenceCanonical:
			route, exists := state.Routes[routeMapKey(ref.Owner)]
			if !exists || route.Canonical.key != key {
				return &Error{Kind: ErrorCorruptState, Field: "canonical", Message: "canonical owner projection is inconsistent"}
			}
		case referenceAlias:
			if err := validateTarget(catalog, state, RouteTarget(ref.Owner)); err != nil {
				return err
			}
		case referenceRedirect:
			if err := validateTarget(catalog, state, ref.Target); err != nil {
				return err
			}
		case referenceGone:
		default:
			return &Error{Kind: ErrorCorruptState, Field: "reference", Message: "unknown reference kind"}
		}
	}
	for key, overlay := range state.Overlays {
		if _, exists := state.Refs[key]; !exists {
			return &Error{Kind: ErrorCorruptState, Field: "overlay", Message: "overlay has no base outcome"}
		}
		if err := validateTarget(catalog, state, overlay.Redirect.Target); err != nil {
			return err
		}
	}
	return nil
}

func validateTarget(catalog *Catalog, state registryState, target Target) error {
	switch target.Kind {
	case TargetRoute:
		if err := catalog.validateRouteKey(target.Route); err != nil {
			return invalid("target.route", "%v", err)
		}
		if _, exists := state.Routes[routeMapKey(target.Route)]; !exists {
			return &Error{Kind: ErrorDanglingTarget, Field: "target.route", Message: "target route has no final canonical"}
		}
	case TargetExternal:
		parsed, err := url.Parse(strings.TrimSpace(target.External))
		if err != nil || !parsed.IsAbs() || parsed.User != nil || parsed.Fragment != "" {
			return &Error{Kind: ErrorExternalForbidden, Field: "target.external", Message: "external target is invalid"}
		}
		origin, _, err := normalizeOrigin(parsed.Scheme + "://" + parsed.Host)
		if err != nil {
			return &Error{Kind: ErrorExternalForbidden, Field: "target.external", Message: "external target origin is invalid"}
		}
		if _, allowed := catalog.externalOrigins[origin]; !allowed {
			return &Error{Kind: ErrorExternalForbidden, Field: "target.external", Message: "external target origin is not allowed"}
		}
	default:
		return invalid("target", "kind is invalid")
	}
	return nil
}

func sortEffects(catalog *Catalog, effects []Effect) {
	slices.SortFunc(effects, func(left, right Effect) int {
		leftRef, _ := catalog.normalizeLocalRef(left.Ref)
		rightRef, _ := catalog.normalizeLocalRef(right.Ref)
		if compared := strings.Compare(leftRef.key, rightRef.key); compared != 0 {
			return compared
		}
		return strings.Compare(string(left.Kind), string(right.Kind))
	})
}

func resolveState(
	catalog *Catalog,
	state registryState,
	lookup normalizedLookup,
	now time.Time,
) (Resolution, error) {
	base, exists := state.Refs[lookup.ref.key]
	if !exists {
		return Resolution{Kind: ResolutionUnknown, Requested: lookup.ref.ref, Revision: state.Revision}, nil
	}
	if overlay, exists := state.Overlays[lookup.ref.key]; exists &&
		(overlay.Redirect.ExpiresAt == nil || now.Before(overlay.Redirect.ExpiresAt.UTC())) {
		resolution, err := redirectResolution(catalog, state, lookup, overlay.Redirect.Target, overlay.Redirect.Policy, overlay.ChangedAt)
		resolution.ExpiresAt = overlay.Redirect.ExpiresAt
		return resolution, err
	}
	switch base.Kind {
	case referenceCanonical:
		route := base.Owner
		canonical := base.Ref.ref
		return Resolution{
			Kind: ResolutionCanonical, Requested: lookup.ref.ref, Route: &route,
			Canonical: &canonical, Revision: state.Revision, ChangedAt: base.ChangedAt,
		}, nil
	case referenceAlias:
		resolution, err := redirectResolution(catalog, state, lookup, RouteTarget(base.Owner), base.Policy, base.ChangedAt)
		resolution.Kind = ResolutionAlias
		return resolution, err
	case referenceRedirect:
		return redirectResolution(catalog, state, lookup, base.Target, base.Policy, base.ChangedAt)
	case referenceGone:
		route := base.Owner
		return Resolution{
			Kind: ResolutionGone, Requested: lookup.ref.ref, Route: &route,
			Revision: state.Revision, ChangedAt: base.ChangedAt,
		}, nil
	default:
		return Resolution{}, &Error{Kind: ErrorCorruptState, Field: "reference", Message: "has an unknown outcome"}
	}
}

func redirectResolution(
	catalog *Catalog,
	state registryState,
	lookup normalizedLookup,
	target Target,
	policy RedirectPolicy,
	changedAt time.Time,
) (Resolution, error) {
	location, canonical, route, err := targetLocation(catalog, state, lookup, target, policy)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{
		Kind: ResolutionRedirect, Requested: lookup.ref.ref, Route: route,
		Canonical: canonical, Location: location, StatusCode: policy.Mode.StatusCode(),
		Revision: state.Revision, ChangedAt: changedAt,
	}, nil
}

func targetLocation(
	catalog *Catalog,
	state registryState,
	lookup normalizedLookup,
	target Target,
	policy RedirectPolicy,
) (string, *LocalRef, *RouteKey, error) {
	switch target.Kind {
	case TargetRoute:
		route, exists := state.Routes[routeMapKey(target.Route)]
		if !exists {
			return "", nil, nil, &Error{Kind: ErrorCorruptState, Field: "target.route", Message: "target route has no canonical"}
		}
		canonical := route.Canonical.ref
		location := applyQueryPolicy(route.Canonical.ref.Path, route.Canonical.query, lookup, policy)
		targetRoute := target.Route
		return location, &canonical, &targetRoute, nil
	case TargetExternal:
		parsed, err := url.Parse(target.External)
		if err != nil {
			return "", nil, nil, &Error{Kind: ErrorCorruptState, Field: "target.external", Message: "cannot parse stored external target", Cause: err}
		}
		baseQuery := parsed.RawQuery
		parsed.RawQuery = ""
		location := applyQueryPolicy(parsed.String(), baseQuery, lookup, policy)
		return location, nil, nil, nil
	default:
		return "", nil, nil, &Error{Kind: ErrorCorruptState, Field: "target", Message: "has an unknown kind"}
	}
}

func applyQueryPolicy(base, canonicalQuery string, lookup normalizedLookup, policy RedirectPolicy) string {
	query := ""
	switch policy.Query {
	case QueryCanonicalWithExtras:
		query = joinQuery(canonicalQuery, lookup.extras)
	case QueryPreserve:
		query = lookup.rawQuery
	case QueryDrop:
	case QueryReplace:
		query = policy.ReplaceQuery
	}
	if query == "" {
		return base
	}
	return base + "?" + query
}

func joinQuery(left, right string) string {
	switch {
	case left == "":
		return right
	case right == "":
		return left
	default:
		return left + "&" + right
	}
}
