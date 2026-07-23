package abuse

import "context"

// Module exposes bound request-path Actions and a separate governance seam.
type Module interface {
	Action(ActionKey) (Action, error)
	Governor() Governor
}

// Action is the only interface ordinary consumers need on request paths.
type Action interface {
	Key() ActionKey
	Admit(context.Context, Input) (Admission, error)
	Resolve(context.Context, Receipt, OutcomeKey) error
}

type Governor interface {
	Actions(context.Context) ([]ActionView, error)
	Inspect(context.Context, InspectQuery) (Inspection, error)
	Reset(context.Context, ResetCommand) (ResetResult, error)
	Prune(context.Context, PruneCommand) (PruneResult, error)
}

// ChallengeVerifier is the only remote port in Abuse. Implementations must
// distinguish an invalid proof from an operational failure.
type ChallengeVerifier interface {
	Verify(context.Context, VerificationRequest) (Verification, error)
}
