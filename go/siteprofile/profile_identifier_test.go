package siteprofile

import (
	"testing"

	"github.com/yueli-official/foundation/go/identifier"
)

func TestNormalizeProfileAssignsStableUUIDv7Identifiers(t *testing.T) {
	existing := identifier.MustNew().String()
	profile := Profile{
		Announcement: Announcement{Action: &Link{ID: "draft-action", Label: "Read", Href: "/news"}},
		Support:      Support{Contacts: []Contact{{ID: existing, Kind: ContactEmail, Label: "Email", Value: "help@example.com"}}},
		Footer: Footer{
			LinkGroups: []LinkGroup{{ID: "draft-group", Title: "About", Links: []Link{{ID: "draft-link", Label: "About", Href: "/about"}}}},
			Social:     []SocialLink{{ID: "draft-social", Platform: "github", URL: "https://github.com/example"}},
			Legal:      []Link{{ID: "draft-legal", Label: "Privacy", Href: "/privacy"}},
			Compliance: Compliance{Records: []ComplianceRecord{{ID: "draft-record", Kind: "icp", Label: "ICP", Number: "Example"}}},
		},
	}

	first := normalizeProfile(profile)
	ids := []string{
		first.Announcement.Action.ID,
		first.Support.Contacts[0].ID,
		first.Footer.LinkGroups[0].ID,
		first.Footer.LinkGroups[0].Links[0].ID,
		first.Footer.Social[0].ID,
		first.Footer.Legal[0].ID,
		first.Footer.Compliance.Records[0].ID,
	}
	seen := map[string]bool{}
	for _, id := range ids {
		value, err := identifier.Parse(id)
		if err != nil || value.Version() != 7 {
			t.Fatalf("normalized ID = %q, error = %v, want UUIDv7", id, err)
		}
		if seen[id] {
			t.Fatalf("normalized ID repeated: %q", id)
		}
		seen[id] = true
	}
	if first.Support.Contacts[0].ID != existing {
		t.Fatalf("existing UUIDv7 = %q, want preserved %q", first.Support.Contacts[0].ID, existing)
	}

	second := normalizeProfile(first)
	if second.Announcement.Action.ID != first.Announcement.Action.ID ||
		second.Footer.LinkGroups[0].Links[0].ID != first.Footer.LinkGroups[0].Links[0].ID {
		t.Fatal("normalizing an already canonical profile changed stable IDs")
	}
}
