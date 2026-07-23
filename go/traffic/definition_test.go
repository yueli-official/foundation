package traffic

import (
	"testing"
	"time"
)

func TestCompileCanonicalAndRejectsUnsafeDefinitions(t *testing.T) {
	left, err := Compile(Definition{
		Version: DefinitionVersion, TimeZone: "UTC",
		ResourceKinds: []ResourceKindDefinition{{Key: "post"}, {Key: "asset"}},
		Policy:        CollectionPolicy{CountedClasses: []VisitClass{VisitHuman, VisitUnknown}},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := Compile(Definition{
		Version: DefinitionVersion, TimeZone: "UTC",
		ResourceKinds: []ResourceKindDefinition{{Key: "asset"}, {Key: "post"}},
		Policy:        CollectionPolicy{CountedClasses: []VisitClass{VisitUnknown, VisitHuman}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if left.Digest() != right.Digest() {
		t.Fatalf("canonical definitions must have the same digest: %s != %s", left.Digest(), right.Digest())
	}
	cases := []Definition{
		{},
		{Version: DefinitionVersion, ResourceKinds: []ResourceKindDefinition{{Key: "Bad Key"}}},
		{Version: DefinitionVersion, ResourceKinds: []ResourceKindDefinition{{Key: "post"}, {Key: "post"}}},
		{Version: DefinitionVersion, TimeZone: "Mars/Olympus", ResourceKinds: []ResourceKindDefinition{{Key: "post"}}},
		{
			Version: DefinitionVersion, ResourceKinds: []ResourceKindDefinition{{Key: "post"}},
			Limits: Limits{MaxPastAge: 48 * time.Hour, ReceiptRetention: 24 * time.Hour},
		},
	}
	for index, definition := range cases {
		if _, err := Compile(definition); !IsKind(err, ErrorInvalidInput) {
			t.Fatalf("case %d: expected invalid input, got %v", index, err)
		}
	}
}

func TestPrepareObservationValidatesPrivacyShapeAndClock(t *testing.T) {
	catalog, err := Compile(Definition{
		Version:       DefinitionVersion,
		ResourceKinds: []ResourceKindDefinition{{Key: "post"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	token, err := VisitorTokenFromBytes(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if !token.IsZero() {
		t.Fatal("zero bytes must produce the zero token")
	}
	_, err = catalog.PrepareObservation(now, Observation{
		EventID: "event-0000000001", Resource: Resource{Kind: "post", ID: "post-a"},
		OccurredAt: now, HasVisitor: true,
	})
	if !IsKind(err, ErrorInvalidInput) {
		t.Fatalf("missing visitor token must fail, got %v", err)
	}
	_, err = catalog.PrepareObservation(now, Observation{
		EventID: "event-0000000001", Resource: Resource{Kind: "post", ID: "post-a"},
		OccurredAt: now.Add(10 * time.Minute),
	})
	if !IsKind(err, ErrorInvalidInput) {
		t.Fatalf("future observation must fail, got %v", err)
	}
}
