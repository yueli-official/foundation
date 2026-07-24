package privacytest

import (
	"context"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/privacy"
)

type RuntimeFactory func(*testing.T, *privacy.Catalog, func() time.Time) privacy.Runtime

func RunRuntime(t *testing.T, factory RuntimeFactory) {
	t.Helper()
	start := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	now := start
	clock := func() time.Time { return now }
	catalog := privacy.MustCompile(Definition(start))
	runtime := factory(t, catalog, clock)
	ctx := context.Background()
	subject := privacy.SubjectRef{Owner: "blog", Kind: "address", Value: "subject@example.test"}
	processing, err := runtime.Purpose(NewsletterPurpose.Key)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := processing.Decide(ctx, privacy.DecisionInput{Subject: privacy.SingleSubject(subject)})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != privacy.DecisionDeny || decision.Reasons[0] != "consent_missing" {
		t.Fatalf("initial decision = %#v", decision)
	}
	receipt, err := runtime.Evidence().Consent(ctx, privacy.ConsentCommand{
		IdempotencyKey: "consent-1", Subject: subject, Notice: NewsletterNotice,
		Purposes: []privacy.PurposeRef{NewsletterPurpose}, OccurredAt: now, Channel: "double_opt_in",
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := runtime.Evidence().Consent(ctx, privacy.ConsentCommand{
		IdempotencyKey: "consent-1", Subject: subject, Notice: NewsletterNotice,
		Purposes: []privacy.PurposeRef{NewsletterPurpose}, OccurredAt: now, Channel: "double_opt_in",
	})
	if err != nil || !replay.Replay || replay.ID != receipt.ID {
		t.Fatalf("consent replay = %#v, %v", replay, err)
	}
	decision, err = processing.Decide(ctx, privacy.DecisionInput{Subject: privacy.SingleSubject(subject)})
	if err != nil || decision.Kind != privacy.DecisionAllow || len(decision.Evidence) != 1 {
		t.Fatalf("consented decision = %#v, %v", decision, err)
	}
	decision, err = processing.Decide(ctx, privacy.DecisionInput{
		Subject: privacy.SingleSubject(subject),
		Signals: []privacy.ObservedSignal{{Signal: "gpc", AssertedAt: now}},
	})
	if err != nil || decision.Kind != privacy.DecisionDeny || decision.Reasons[0] != "privacy_signal" {
		t.Fatalf("GPC decision = %#v, %v", decision, err)
	}
	now = now.Add(time.Minute)
	if _, err := runtime.Evidence().Withdraw(ctx, privacy.WithdrawalCommand{
		IdempotencyKey: "withdraw-1", Subject: subject, Purposes: []privacy.PurposeRef{NewsletterPurpose},
		OccurredAt: now, Channel: "self_service",
	}); err != nil {
		t.Fatal(err)
	}
	decision, err = processing.Decide(ctx, privacy.DecisionInput{Subject: privacy.SingleSubject(subject)})
	if err != nil || decision.Kind != privacy.DecisionDeny || decision.Reasons[0] != "consent_withdrawn" {
		t.Fatalf("withdrawn decision = %#v, %v", decision, err)
	}
	security, err := runtime.Purpose(SecurityPurpose.Key)
	if err != nil {
		t.Fatal(err)
	}
	decision, err = security.Decide(ctx, privacy.DecisionInput{Subject: privacy.SingleSubject(subject)})
	if err != nil || decision.Kind != privacy.DecisionAllow || decision.Reasons[0] != "declared_non_consent_basis" {
		t.Fatalf("security decision = %#v, %v", decision, err)
	}
	item, err := runtime.Retention().Track(ctx, privacy.RetentionCommand{
		IdempotencyKey: "retention-1", Record: privacy.RecordRef{Dataset: "blog.comments", Value: "comment-1"},
		Rule: CommentRetention, TriggeredAt: start,
	})
	if err != nil {
		t.Fatal(err)
	}
	now = start.AddDate(0, 0, 31)
	page, err := runtime.Retention().Due(ctx, privacy.RetentionDueQuery{At: now, Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].State != privacy.RetentionDue {
		t.Fatalf("due page = %#v, %v", page, err)
	}
	next := now.AddDate(0, 0, 30)
	item, err = runtime.Retention().Review(ctx, privacy.RetentionReviewCommand{
		IdempotencyKey: "review-1", ItemID: item.ID, Outcome: privacy.DispositionRetained,
		Reason: "legal_hold", ReviewAfter: &next,
	})
	if err != nil || item.State != privacy.RetentionRetained || !item.ReviewAt.Equal(next) {
		t.Fatalf("retention review = %#v, %v", item, err)
	}
}

type CoordinatorFactory func(
	*testing.T,
	*privacy.Catalog,
	func() time.Time,
	privacy.OwnerRouter,
) privacy.Coordinator

func RunCoordinator(t *testing.T, factory CoordinatorFactory) {
	t.Helper()
	start := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	now := start
	clock := func() time.Time { return now }
	catalog := privacy.MustCompile(Definition(start))
	owners := catalog.Owners()
	hosts := map[privacy.OwnerKey]privacy.OwnerHost{}
	var executionOrder []privacy.OwnerKey
	for _, owner := range owners {
		definition := owner
		host, err := privacy.NewMemoryOwnerHost(definition, privacy.MemoryOwnerHostOptions{
			Clock: clock,
			Executor: privacy.OwnerExecutorFunc(func(_ context.Context, instruction privacy.OwnerInstruction) (privacy.OwnerOutcome, error) {
				executionOrder = append(executionOrder, definition.Ref.Key)
				results := make([]privacy.DatasetOutcome, 0, len(instruction.Command.Datasets))
				for _, dataset := range instruction.Command.Datasets {
					outcome := privacy.DatasetOutcome{Dataset: dataset, Disposition: privacy.DispositionDeleted, Count: 1}
					if definition.Ref.Key == "blog" && dataset == "blog.comments" {
						review := now.AddDate(0, 0, 30)
						outcome = privacy.DatasetOutcome{
							Dataset: dataset, Disposition: privacy.DispositionRetained, Count: 1,
							Reason: "public_authorship", ReviewAfter: &review,
						}
					}
					results = append(results, outcome)
				}
				return privacy.OwnerOutcome{Terminal: true, Results: results}, nil
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		hosts[definition.Ref.Key] = host
	}
	router := privacy.OwnerRouterFunc(func(_ context.Context, key privacy.OwnerKey) (privacy.OwnerHost, error) {
		return hosts[key], nil
	})
	coordinator := factory(t, catalog, clock, router)
	ctx := context.Background()
	subject := privacy.SubjectRef{Owner: "identity", Kind: "user", Value: "user-1"}
	command := privacy.OpenRightsRequest{
		IdempotencyKey: "request-1", Subject: privacy.SingleSubject(subject),
		Operation: privacy.RightErasure, RequestedAt: now, Channel: "self_service",
		Verification: privacy.VerificationEvidence{
			VerifiedAt: now, Method: "session_reauthentication", Assurance: "high", VerificationRef: "verify-1",
		},
	}
	view, err := coordinator.Open(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Tasks) != 2 || view.Phase != privacy.RequestOpen {
		t.Fatalf("open view = %#v", view)
	}
	replay, err := coordinator.Open(ctx, command)
	if err != nil || !replay.Replay || replay.ID != view.ID {
		t.Fatalf("request replay = %#v, %v", replay, err)
	}
	future := command
	future.IdempotencyKey = "request-future"
	future.RequestedAt = now.Add(time.Hour)
	future.Verification.VerifiedAt = future.RequestedAt
	if _, err := coordinator.Open(ctx, future); err == nil {
		t.Fatal("future verification and request time were accepted")
	}
	driven, err := coordinator.Drive(ctx, privacy.DriveRightsRequest{
		Request: view.ID, Budget: privacy.DriveBudget{MaxOwnerAttempts: 8, MaxDuration: time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if driven.View.Phase != privacy.RequestComplete || driven.View.Summary.Retained != 1 ||
		driven.View.Summary.Performed != 2 || driven.View.Summary.Pending != 0 {
		t.Fatalf("driven view = %#v", driven.View)
	}
	if len(executionOrder) != 2 || executionOrder[0] != "blog" || executionOrder[1] != "identity" {
		t.Fatalf("finalizing owner execution order = %v", executionOrder)
	}
}
