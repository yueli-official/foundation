package audit_test

import (
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/audit"
)

func TestCompileRejectsSensitiveEvidenceAndMutableActionVersions(t *testing.T) {
	_, err := audit.Compile(audit.Definition{
		Version: 1, Consumer: "docs.main",
		Retention: []audit.RetentionDefinition{
			{Class: "retention.management", MinimumAge: time.Hour},
		},
		Actions: []audit.ActionDefinition{
			{
				Action:      audit.Action{Name: "docs.document.published", Version: 1},
				TargetTypes: []string{"docs.document"}, Retention: "retention.management",
				Evidence: []audit.FieldDefinition{
					{Key: "request.access_token", Kind: audit.EvidenceReference},
				},
			},
		},
	})
	if !audit.IsKind(err, audit.ErrorInvalidDefinition) {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareRejectsUnknownEvidenceAndInvalidTrace(t *testing.T) {
	catalog := audit.MustCompile(audit.Definition{
		Version: 1, Consumer: "docs.main",
		Retention: []audit.RetentionDefinition{
			{Class: "retention.management", MinimumAge: time.Hour},
		},
		Actions: []audit.ActionDefinition{
			{
				Action:      audit.Action{Name: "docs.document.published", Version: 1},
				TargetTypes: []string{"docs.document"}, Retention: "retention.management",
			},
		},
	})
	contract := audit.MustBindAction(catalog, audit.Action{Name: "docs.document.published", Version: 1}, func(string) []audit.EvidenceField {
		return []audit.EvidenceField{audit.Code("unexpected.value", "x")}
	})
	_, err := audit.Prepare(contract, audit.Attempt[string]{
		ID: "event-1", Actor: audit.Actor{Kind: audit.ActorUser, ID: "user-1"},
		Target:      audit.Target{Type: "docs.document", ID: "doc-1"},
		Outcome:     audit.Outcome{Kind: audit.OutcomeSucceeded},
		Correlation: audit.Correlation{SpanID: "0123456789abcdef"},
	})
	if !audit.IsKind(err, audit.ErrorInvalidAttempt) {
		t.Fatalf("error = %v", err)
	}
}
