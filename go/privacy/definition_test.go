package privacy_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/privacy"
	"github.com/yueli-official/foundation/go/privacy/privacytest"
)

func TestDefinitionCanonicalizesSetOrdering(t *testing.T) {
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	first := privacytest.Definition(now)
	second := privacytest.Definition(now)
	slices.Reverse(second.SubjectKinds)
	slices.Reverse(second.DataCategories)
	slices.Reverse(second.Purposes)
	slices.Reverse(second.ActivePurposes)
	slices.Reverse(second.Coordination.Owners)
	for index := range second.Coordination.Owners {
		slices.Reverse(second.Coordination.Owners[index].Datasets)
	}
	a, err := privacy.Compile(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := privacy.Compile(second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest() != b.Digest() {
		t.Fatalf("catalog digest changes with set ordering: %s != %s", a.Digest(), b.Digest())
	}
}

func TestMemoryOwnerHostReplaysTerminalReceipt(t *testing.T) {
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	catalog := privacy.MustCompile(privacytest.Definition(now))
	owner, ok := catalog.Owner()
	if !ok {
		t.Fatal("local owner is missing")
	}
	calls := 0
	host, err := privacy.NewMemoryOwnerHost(owner, privacy.MemoryOwnerHostOptions{
		Clock: func() time.Time { return now },
		Executor: privacy.OwnerExecutorFunc(func(_ context.Context, instruction privacy.OwnerInstruction) (privacy.OwnerOutcome, error) {
			calls++
			results := make([]privacy.DatasetOutcome, 0, len(instruction.Command.Datasets))
			for _, dataset := range instruction.Command.Datasets {
				results = append(results, privacy.DatasetOutcome{Dataset: dataset, Disposition: privacy.DispositionDeleted})
			}
			return privacy.OwnerOutcome{Terminal: true, Results: results}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	command := privacy.OwnerCommand{
		ProtocolVersion: privacy.OwnerProtocolVersion,
		RequestID:       "request-1", TaskID: "task-1", Owner: owner.Ref,
		Operation:   privacy.RightErasure,
		Subject:     privacy.SingleSubject(privacy.SubjectRef{Owner: "identity", Kind: "user", Value: "user-1"}),
		Datasets:    []privacy.DatasetKey{"blog.comments", "blog.newsletter"},
		RequestedAt: now, Deadline: now.AddDate(0, 0, 30),
	}
	// Fingerprint is protocol-owned; obtain a real command by opening a request
	// through the Coordinator rather than duplicating its canonicalization here.
	hosts := map[privacy.OwnerKey]privacy.OwnerHost{owner.Ref.Key: host}
	for _, value := range catalog.Owners() {
		if value.Ref.Key == owner.Ref.Key {
			continue
		}
		fallback, createErr := privacy.NewMemoryOwnerHost(value, privacy.MemoryOwnerHostOptions{
			Executor: privacy.OwnerExecutorFunc(func(_ context.Context, instruction privacy.OwnerInstruction) (privacy.OwnerOutcome, error) {
				results := make([]privacy.DatasetOutcome, 0, len(instruction.Command.Datasets))
				for _, dataset := range instruction.Command.Datasets {
					results = append(results, privacy.DatasetOutcome{Dataset: dataset, Disposition: privacy.DispositionNotFound})
				}
				return privacy.OwnerOutcome{Terminal: true, Results: results}, nil
			}),
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		hosts[value.Ref.Key] = fallback
	}
	router := privacy.OwnerRouterFunc(func(_ context.Context, key privacy.OwnerKey) (privacy.OwnerHost, error) {
		return hosts[key], nil
	})
	coordinator, err := privacy.NewMemoryCoordinator(catalog, privacy.MemoryCoordinatorOptions{Clock: func() time.Time { return now }, Router: router})
	if err != nil {
		t.Fatal(err)
	}
	view, err := coordinator.Open(context.Background(), privacy.OpenRightsRequest{
		IdempotencyKey: "request-1", Subject: command.Subject, Operation: privacy.RightErasure,
		RequestedAt: now, Channel: "test",
		Verification: privacy.VerificationEvidence{VerifiedAt: now, Method: "test", VerificationRef: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Drive(context.Background(), privacy.DriveRightsRequest{
		Request: view.ID, Budget: privacy.DriveBudget{MaxOwnerAttempts: 8, MaxDuration: time.Second},
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("executor calls = %d, want 1", calls)
	}
}
