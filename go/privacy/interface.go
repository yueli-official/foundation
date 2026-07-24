package privacy

import "context"

type Runtime interface {
	Purpose(PurposeKey) (Processing, error)
	Evidence() EvidenceLedger
	Retention() RetentionLedger
}

type Processing interface {
	Ref() PurposeRef
	Decide(context.Context, DecisionInput) (ProcessingDecision, error)
}

type EvidenceLedger interface {
	Consent(context.Context, ConsentCommand) (ConsentReceipt, error)
	Withdraw(context.Context, WithdrawalCommand) (WithdrawalReceipt, error)
	ObserveSignal(context.Context, SignalCommand) (SignalReceipt, error)
}

type RetentionLedger interface {
	Track(context.Context, RetentionCommand) (RetentionItem, error)
	Review(context.Context, RetentionReviewCommand) (RetentionItem, error)
	Due(context.Context, RetentionDueQuery) (RetentionPage, error)
}

type Coordinator interface {
	Open(context.Context, OpenRightsRequest) (RightsRequestView, error)
	Drive(context.Context, DriveRightsRequest) (DriveResult, error)
	Get(context.Context, RightsRequestID) (RightsRequestView, error)
}

type OwnerHost interface {
	Handle(context.Context, OwnerCommand) (OwnerReceipt, error)
}

type OwnerRouter interface {
	Owner(context.Context, OwnerKey) (OwnerHost, error)
}

type OwnerRouterFunc func(context.Context, OwnerKey) (OwnerHost, error)

func (function OwnerRouterFunc) Owner(ctx context.Context, key OwnerKey) (OwnerHost, error) {
	return function(ctx, key)
}

type OwnerExecutor interface {
	Execute(context.Context, OwnerInstruction) (OwnerOutcome, error)
}

type OwnerExecutorFunc func(context.Context, OwnerInstruction) (OwnerOutcome, error)

func (function OwnerExecutorFunc) Execute(ctx context.Context, instruction OwnerInstruction) (OwnerOutcome, error) {
	return function(ctx, instruction)
}
