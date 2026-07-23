package urllifecycle

import "time"

type referenceKind string

const (
	referenceCanonical referenceKind = "canonical"
	referenceAlias     referenceKind = "alias"
	referenceRedirect  referenceKind = "redirect"
	referenceGone      referenceKind = "gone"
)

type storedRoute struct {
	Key       RouteKey
	Canonical normalizedRef
	Aliases   map[string]storedAlias
	Revision  RouteRevision
	ChangedAt time.Time
}

type storedAlias struct {
	Ref    normalizedRef
	Policy RedirectPolicy
}

type storedReference struct {
	Ref       normalizedRef
	Kind      referenceKind
	Owner     RouteKey
	Target    Target
	Policy    RedirectPolicy
	ChangedAt time.Time
}

type storedOverlay struct {
	Owner     RouteKey
	Source    normalizedRef
	Redirect  TemporaryRedirect
	ChangedAt time.Time
}

type storedCommand struct {
	Digest  Digest
	Receipt Receipt
}

type registryState struct {
	Revision Revision
	Routes   map[string]storedRoute
	Refs     map[string]storedReference
	Overlays map[string]storedOverlay
	Commands map[CommandID]storedCommand
	History  []TransitionSummary
}

func emptyState() registryState {
	return registryState{
		Routes: map[string]storedRoute{}, Refs: map[string]storedReference{},
		Overlays: map[string]storedOverlay{}, Commands: map[CommandID]storedCommand{},
	}
}

func (state registryState) clone() registryState {
	copyState := registryState{
		Revision: state.Revision,
		Routes:   make(map[string]storedRoute, len(state.Routes)),
		Refs:     make(map[string]storedReference, len(state.Refs)),
		Overlays: make(map[string]storedOverlay, len(state.Overlays)),
		Commands: make(map[CommandID]storedCommand, len(state.Commands)),
		History:  append([]TransitionSummary(nil), state.History...),
	}
	for key, route := range state.Routes {
		route.Aliases = cloneAliases(route.Aliases)
		copyState.Routes[key] = route
	}
	for key, ref := range state.Refs {
		copyState.Refs[key] = ref
	}
	for key, overlay := range state.Overlays {
		copyState.Overlays[key] = overlay
	}
	for key, command := range state.Commands {
		copyState.Commands[key] = command
	}
	return copyState
}

func cloneAliases(values map[string]storedAlias) map[string]storedAlias {
	result := make(map[string]storedAlias, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func routeMapKey(value RouteKey) string {
	return string(value.Resource.Kind) + "\x00" + value.Resource.ID + "\x00" + value.Variant
}
