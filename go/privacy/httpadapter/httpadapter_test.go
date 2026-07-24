package httpadapter_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/privacy"
	"github.com/yueli-official/foundation/go/privacy/httpadapter"
	"github.com/yueli-official/foundation/go/privacy/privacytest"
)

func TestOwnerRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	definition := privacytest.Definition(now)
	definition.Coordination.Owners = []privacy.OwnerDefinition{*definition.Owner}
	catalog := privacy.MustCompile(definition)
	owner, ok := catalog.Owner()
	if !ok {
		t.Fatal("owner missing")
	}
	host, err := privacy.NewMemoryOwnerHost(owner, privacy.MemoryOwnerHostOptions{
		Clock: func() time.Time { return now },
		Executor: privacy.OwnerExecutorFunc(func(_ context.Context, instruction privacy.OwnerInstruction) (privacy.OwnerOutcome, error) {
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
	handler, err := httpadapter.NewHandler(httpadapter.HandlerOptions{Host: host})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := httpadapter.NewClient(httpadapter.ClientOptions{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	command := privacy.OwnerCommand{
		ProtocolVersion: privacy.OwnerProtocolVersion, RequestID: "request", TaskID: "task",
		Owner: owner.Ref, Operation: privacy.RightErasure,
		Subject:     privacy.SingleSubject(privacy.SubjectRef{Owner: "identity", Kind: "user", Value: "user"}),
		Datasets:    []privacy.DatasetKey{"blog.comments", "blog.newsletter"},
		RequestedAt: now, Deadline: now.Add(time.Hour),
	}
	// Open/Drive owns command fingerprints; use a one-owner coordinator to
	// exercise the real protocol rather than duplicating its private digest.
	router := privacy.OwnerRouterFunc(func(context.Context, privacy.OwnerKey) (privacy.OwnerHost, error) { return client, nil })
	coordinator, err := privacy.NewMemoryCoordinator(catalog, privacy.MemoryCoordinatorOptions{Clock: func() time.Time { return now }, Router: router})
	if err != nil {
		t.Fatal(err)
	}
	view, err := coordinator.Open(context.Background(), privacy.OpenRightsRequest{
		IdempotencyKey: "request", Subject: command.Subject, Operation: privacy.RightErasure,
		RequestedAt: now, Channel: "test",
		Verification: privacy.VerificationEvidence{VerifiedAt: now, Method: "test", VerificationRef: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Drive(context.Background(), privacy.DriveRightsRequest{
		Request: view.ID, Budget: privacy.DriveBudget{MaxOwnerAttempts: 8, MaxDuration: time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.View.Phase != privacy.RequestComplete {
		t.Fatalf("phase = %s", result.View.Phase)
	}
}

func TestClientRejectsInsecureRemoteEndpoint(t *testing.T) {
	if _, err := httpadapter.NewClient(httpadapter.ClientOptions{Endpoint: "http://example.com/privacy"}); err == nil {
		t.Fatal("expected insecure endpoint error")
	}
}

func TestClientAllowsExplicitIsolatedNetworkHTTP(t *testing.T) {
	if _, err := httpadapter.NewClient(httpadapter.ClientOptions{
		Endpoint: "http://blog-api:8085/privacy", AllowInsecureHTTP: true,
	}); err != nil {
		t.Fatalf("explicit isolated-network endpoint: %v", err)
	}
}
