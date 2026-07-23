package abusetest

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/abuse"
)

type Factory func(
	t *testing.T,
	catalog *abuse.Catalog,
	clock func() time.Time,
	verifiers map[abuse.ChallengeKind]abuse.ChallengeVerifier,
) abuse.Module

type acceptingVerifier struct {
	calls int
}

func (verifier *acceptingVerifier) Verify(_ context.Context, request abuse.VerificationRequest) (abuse.Verification, error) {
	verifier.calls++
	if request.Token != "accepted" {
		return abuse.Verification{Status: abuse.VerificationRejected}, nil
	}
	return abuse.Verification{
		Status: abuse.VerificationAccepted, Action: request.ExpectedAction,
		Hostname: request.AllowedHosts[0],
	}, nil
}

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("atomic_multi_meter_and_idempotency", func(t *testing.T) {
		now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
		catalog := abuse.MustCompile(abuse.Definition{
			Version: 1, Consumer: "conformance",
			Actions: []abuse.ActionDefinition{{
				Key:      "test.write",
				Required: abuse.SignalRequirements{Network: abuse.Required, Target: abuse.Required},
				Meters: []abuse.MeterDefinition{
					{ID: "write.network", Slot: abuse.SlotNetwork, Algorithm: abuse.FixedWindow(2, time.Hour)},
					{ID: "write.target", Slot: abuse.SlotTarget, Algorithm: abuse.FixedWindow(1, time.Hour)},
				},
			}},
		})
		module := factory(t, catalog, func() time.Time { return now }, nil)
		action := mustAction(t, module, "test.write")
		input := abuse.Input{
			ID: "attempt-00000001",
			Signals: abuse.Signals{
				Network: netip.MustParsePrefix("192.0.2.4/32"),
				Target:  "target-a",
			},
		}
		first := mustAdmit(t, action, input)
		if first.Disposition != abuse.DispositionAllow || first.Replay {
			t.Fatalf("first admission = %+v", first)
		}
		replay := mustAdmit(t, action, input)
		if replay.Disposition != abuse.DispositionAllow || !replay.Replay {
			t.Fatalf("replay admission = %+v", replay)
		}
		input.ID = "attempt-00000002"
		second := mustAdmit(t, action, input)
		if second.Disposition != abuse.DispositionReject {
			t.Fatalf("second admission = %+v", second)
		}
		inspection, err := module.Governor().Inspect(context.Background(), abuse.InspectQuery{
			Action: "test.write", Signals: input.Signals,
		})
		if err != nil {
			t.Fatal(err)
		}
		used := map[abuse.PolicyID]int64{}
		for _, meter := range inspection.Meters {
			used[meter.Policy] = meter.Used
		}
		if used["write.network"] != 1 || used["write.target"] != 1 {
			t.Fatalf("second denial partially consumed: %+v", used)
		}
		conflicting := input
		conflicting.Signals.Target = "target-b"
		if _, err := action.Admit(context.Background(), conflicting); !abuse.IsKind(err, abuse.ErrorConflict) {
			t.Fatalf("conflicting replay error = %v", err)
		}
	})

	t.Run("challenge_continuation_and_hard_reject", func(t *testing.T) {
		now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
		verifier := &acceptingVerifier{}
		catalog := abuse.MustCompile(abuse.Definition{
			Version: 1, Consumer: "conformance",
			Actions: []abuse.ActionDefinition{{
				Key:      "test.challenge",
				Required: abuse.SignalRequirements{Network: abuse.Required},
				Meters: []abuse.MeterDefinition{{
					ID: "challenge.network", Slot: abuse.SlotNetwork,
					Algorithm: abuse.SlidingWindow(3, time.Hour), ChallengeAt: 2,
				}},
				Challenge: &abuse.ChallengeDefinition{
					Kind: "turnstile", ExpectedAction: "test-challenge",
					AllowedHosts: []string{"example.test"},
				},
			}},
		})
		module := factory(t, catalog, func() time.Time { return now }, map[abuse.ChallengeKind]abuse.ChallengeVerifier{
			"turnstile": verifier,
		})
		action := mustAction(t, module, "test.challenge")
		signals := abuse.Signals{Network: netip.MustParsePrefix("198.51.100.9/32")}
		if got := mustAdmit(t, action, abuse.Input{ID: "challenge-000001", Signals: signals}); got.Disposition != abuse.DispositionAllow {
			t.Fatalf("first = %+v", got)
		}
		input := abuse.Input{ID: "challenge-000002", Signals: signals}
		challenged := mustAdmit(t, action, input)
		if challenged.Disposition != abuse.DispositionChallenge || challenged.Challenge == nil {
			t.Fatalf("challenge = %+v", challenged)
		}
		input.Proof = &abuse.Proof{Kind: "turnstile", Token: "accepted"}
		allowed := mustAdmit(t, action, input)
		if allowed.Disposition != abuse.DispositionAllow || verifier.calls != 1 {
			t.Fatalf("continued = %+v calls=%d", allowed, verifier.calls)
		}
		third := mustAdmit(t, action, abuse.Input{
			ID: "challenge-000003", Signals: signals,
			Proof: &abuse.Proof{Kind: "turnstile", Token: "accepted"},
		})
		if third.Disposition != abuse.DispositionAllow {
			t.Fatalf("third = %+v", third)
		}
		fourth := mustAdmit(t, action, abuse.Input{
			ID: "challenge-000004", Signals: signals,
			Proof: &abuse.Proof{Kind: "turnstile", Token: "accepted"},
		})
		if fourth.Disposition != abuse.DispositionReject {
			t.Fatalf("fourth = %+v", fourth)
		}
		if verifier.calls != 2 {
			t.Fatalf("reject must not call verifier; calls=%d", verifier.calls)
		}
	})

	t.Run("outcome_pending_and_success_reset", func(t *testing.T) {
		now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
		catalog := abuse.MustCompile(abuse.Definition{
			Version: 1, Consumer: "conformance",
			Actions: []abuse.ActionDefinition{{
				Key:      "test.login",
				Required: abuse.SignalRequirements{Target: abuse.Required},
				Meters: []abuse.MeterDefinition{{
					ID: "login.target", Slot: abuse.SlotTarget, Mode: abuse.MeterOutcome,
					Algorithm: abuse.SlidingWindow(5, time.Hour),
					ChargeOn:  []abuse.OutcomeKey{"credentials_rejected"},
					ResetOn:   []abuse.OutcomeKey{"authenticated"},
				}},
				Resolution: &abuse.ResolutionDefinition{
					Outcomes:       []abuse.OutcomeKey{"authenticated", "credentials_rejected"},
					DefaultOutcome: "credentials_rejected", PendingTTL: time.Minute,
				},
			}},
		})
		module := factory(t, catalog, func() time.Time { return now }, nil)
		action := mustAction(t, module, "test.login")
		signals := abuse.Signals{Target: "canonical@example.test"}
		first := mustAdmit(t, action, abuse.Input{ID: "login-attempt-001", Signals: signals})
		second := mustAdmit(t, action, abuse.Input{ID: "login-attempt-002", Signals: signals})
		if err := action.Resolve(context.Background(), first.Receipt, "credentials_rejected"); err != nil {
			t.Fatal(err)
		}
		if err := action.Resolve(context.Background(), second.Receipt, "authenticated"); err != nil {
			t.Fatal(err)
		}
		assertUsed(t, module, "test.login", signals, "login.target", 0)

		olderPending := mustAdmit(t, action, abuse.Input{ID: "login-attempt-003", Signals: signals})
		success := mustAdmit(t, action, abuse.Input{ID: "login-attempt-004", Signals: signals})
		if err := action.Resolve(context.Background(), success.Receipt, "authenticated"); err != nil {
			t.Fatal(err)
		}
		assertUsed(t, module, "test.login", signals, "login.target", 1)
		if err := action.Resolve(context.Background(), olderPending.Receipt, "credentials_rejected"); err != nil {
			t.Fatal(err)
		}
		assertUsed(t, module, "test.login", signals, "login.target", 1)
		if err := action.Resolve(context.Background(), olderPending.Receipt, "authenticated"); !abuse.IsKind(err, abuse.ErrorConflict) {
			t.Fatalf("conflicting resolution error = %v", err)
		}
	})

	t.Run("pending_defaults_conservatively", func(t *testing.T) {
		now := time.Date(2026, 7, 24, 11, 0, 0, 0, time.UTC)
		catalog := abuse.MustCompile(abuse.Definition{
			Version: 1, Consumer: "conformance",
			Actions: []abuse.ActionDefinition{{
				Key:      "test.default",
				Required: abuse.SignalRequirements{Target: abuse.Required},
				Meters: []abuse.MeterDefinition{{
					ID: "default.target", Slot: abuse.SlotTarget, Mode: abuse.MeterOutcome,
					Algorithm: abuse.SlidingWindow(2, time.Hour),
					ChargeOn:  []abuse.OutcomeKey{"failed"},
				}},
				Resolution: &abuse.ResolutionDefinition{
					Outcomes:       []abuse.OutcomeKey{"ok", "failed"},
					DefaultOutcome: "failed", PendingTTL: time.Minute,
				},
			}},
		})
		module := factory(t, catalog, func() time.Time { return now }, nil)
		action := mustAction(t, module, "test.default")
		signals := abuse.Signals{Target: "target"}
		mustAdmit(t, action, abuse.Input{ID: "default-attempt01", Signals: signals})
		now = now.Add(2 * time.Minute)
		if _, err := module.Governor().Prune(context.Background(), abuse.PruneCommand{
			Before: now.Add(time.Hour), Limit: 100,
		}); err != nil {
			t.Fatal(err)
		}
		assertUsed(t, module, "test.default", signals, "default.target", 1)
	})

	t.Run("token_bucket_preserves_partial_refill", func(t *testing.T) {
		now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
		catalog := abuse.MustCompile(abuse.Definition{
			Version: 1, Consumer: "conformance",
			Actions: []abuse.ActionDefinition{{
				Key:      "test.token",
				Required: abuse.SignalRequirements{Network: abuse.Required},
				Meters: []abuse.MeterDefinition{{
					ID: "token.network", Slot: abuse.SlotNetwork,
					Algorithm: abuse.TokenBucket(2, 1, 10*time.Second),
				}},
			}},
		})
		module := factory(t, catalog, func() time.Time { return now }, nil)
		action := mustAction(t, module, "test.token")
		signals := abuse.Signals{Network: netip.MustParsePrefix("192.0.2.80/32")}
		mustAdmit(t, action, abuse.Input{ID: "token-attempt-001", Signals: signals})
		mustAdmit(t, action, abuse.Input{ID: "token-attempt-002", Signals: signals})
		denied := mustAdmit(t, action, abuse.Input{ID: "token-attempt-003", Signals: signals})
		if denied.Disposition != abuse.DispositionReject || denied.RetryAt != now.Add(10*time.Second) {
			t.Fatalf("initial denial=%+v", denied)
		}
		now = now.Add(5 * time.Second)
		denied = mustAdmit(t, action, abuse.Input{ID: "token-attempt-004", Signals: signals})
		if denied.Disposition != abuse.DispositionReject || denied.RetryAt != now.Add(5*time.Second) {
			t.Fatalf("partial denial=%+v", denied)
		}
		now = now.Add(5 * time.Second)
		allowed := mustAdmit(t, action, abuse.Input{ID: "token-attempt-005", Signals: signals})
		if allowed.Disposition != abuse.DispositionAllow {
			t.Fatalf("refilled admission=%+v", allowed)
		}
	})
}

func mustAction(t *testing.T, module abuse.Module, key abuse.ActionKey) abuse.Action {
	t.Helper()
	action, err := module.Action(key)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func mustAdmit(t *testing.T, action abuse.Action, input abuse.Input) abuse.Admission {
	t.Helper()
	admission, err := action.Admit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return admission
}

func assertUsed(
	t *testing.T,
	module abuse.Module,
	action abuse.ActionKey,
	signals abuse.Signals,
	policy abuse.PolicyID,
	expected int64,
) {
	t.Helper()
	inspection, err := module.Governor().Inspect(context.Background(), abuse.InspectQuery{
		Action: action, Signals: signals,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, meter := range inspection.Meters {
		if meter.Policy == policy {
			if meter.Used != expected {
				t.Fatalf("%s used=%d want %d", policy, meter.Used, expected)
			}
			return
		}
	}
	t.Fatalf("policy %s was not inspected", policy)
}
