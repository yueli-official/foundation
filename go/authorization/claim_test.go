package authorization_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestMemoryInitialAdministratorClaim(t *testing.T) {
	module, err := authorization.NewMemory(
		authorization.MustCompile(validDefinition()),
		authorization.MemoryOptions{RootScopeID: "docs", AllowUnclaimed: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before, err := module.AdministratorClaimStatus(ctx)
	if err != nil || before.Claimed {
		t.Fatalf("before = %#v, err = %v", before, err)
	}
	claimant := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "owner"}
	claimed, err := module.ClaimInitialAdministrator(ctx, authorization.ClaimInitialAdministratorCommand{Actor: claimant})
	if err != nil {
		t.Fatal(err)
	}
	if !claimed.Created || !claimed.Status.Claimed || claimed.Grant.Target != claimant ||
		claimed.Grant.Source != authorization.GrantSourceInitialClaim {
		t.Fatalf("claim = %#v", claimed)
	}
	decision, err := module.Decide(ctx, authorization.DecisionRequest{
		Subject: claimant, Capability: authorization.CapabilityManage, ScopeID: "docs",
	})
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %#v, err = %v", decision, err)
	}
	repeated, err := module.ClaimInitialAdministrator(ctx, authorization.ClaimInitialAdministratorCommand{Actor: claimant})
	if err != nil || repeated.Created || repeated.Grant.ID != claimed.Grant.ID {
		t.Fatalf("repeated = %#v, err = %v", repeated, err)
	}
	other := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "other"}
	if _, err := module.ClaimInitialAdministrator(ctx, authorization.ClaimInitialAdministratorCommand{Actor: other}); !authorization.Is(err, authorization.ErrorConflict) {
		t.Fatalf("other claimant error = %v", err)
	}
	audit, err := module.SearchAudit(ctx, authorization.AuditQuery{
		Actor: claimant, Action: authorization.AuditInitialAdministratorClaimed, ScopeID: "docs",
	})
	if err != nil || audit.Total != 1 || audit.Events[0].Subject != claimant {
		t.Fatalf("audit = %#v, err = %v", audit, err)
	}
}

func TestMemoryInitialAdministratorClaimHasOneConcurrentWinner(t *testing.T) {
	module, err := authorization.NewMemory(
		authorization.MustCompile(validDefinition()),
		authorization.MemoryOptions{RootScopeID: "docs", AllowUnclaimed: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	const contenders = 20
	var wait sync.WaitGroup
	results := make(chan error, contenders)
	for index := 0; index < contenders; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := module.ClaimInitialAdministrator(context.Background(), authorization.ClaimInitialAdministratorCommand{
				Actor: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: fmt.Sprintf("user-%02d", index)},
			})
			results <- err
		}(index)
	}
	wait.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case authorization.Is(err, authorization.ErrorConflict):
			conflicts++
		default:
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if success != 1 || conflicts != contenders-1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
}

func TestMemoryBootstrapClosesInitialAdministratorClaim(t *testing.T) {
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "bootstrap-admin"}
	module, err := authorization.NewMemory(
		authorization.MustCompile(validDefinition()),
		authorization.MemoryOptions{
			RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin}, AllowUnclaimed: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	status, err := module.AdministratorClaimStatus(context.Background())
	if err != nil || !status.Claimed {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
	if _, err := module.ClaimInitialAdministrator(context.Background(), authorization.ClaimInitialAdministratorCommand{
		Actor: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "other"},
	}); !authorization.Is(err, authorization.ErrorConflict) {
		t.Fatalf("claim after bootstrap error = %v", err)
	}
}
