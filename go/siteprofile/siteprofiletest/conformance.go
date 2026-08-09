package siteprofiletest

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/siteprofile"
)

type Factory func(t *testing.T, clock siteprofile.Clock) siteprofile.Module

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("not initialized", func(t *testing.T) {
		module := factory(t, fixedClock{value: instant()})
		if _, err := module.Get(context.Background()); !errors.Is(err, siteprofile.ErrNotInitialized) {
			t.Fatalf("Get error = %v, want ErrNotInitialized", err)
		}
	})
	t.Run("bootstrap normalizes and emits stable metadata", func(t *testing.T) {
		module := factory(t, fixedClock{value: instant()})
		result, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{
			ExpectedRevision: 0,
			Profile:          validProfile(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Changed || result.Snapshot.Revision != 1 {
			t.Fatalf("result = %#v", result)
		}
		if result.Snapshot.Profile.Identity.Name != "Example Site" {
			t.Fatalf("normalized name = %q", result.Snapshot.Profile.Identity.Name)
		}
		if result.Snapshot.ETag == "" || result.Snapshot.DocumentDigest == "" || result.Snapshot.SchemaVersion != 1 {
			t.Fatalf("metadata = %#v", result.Snapshot)
		}
		again, err := module.Get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if again.ETag != result.Snapshot.ETag || again.DocumentDigest != result.Snapshot.DocumentDigest {
			t.Fatalf("Get metadata drifted: %#v != %#v", again, result.Snapshot)
		}
	})
	t.Run("same normalized document is a no-op", func(t *testing.T) {
		module := factory(t, fixedClock{value: instant()})
		first, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{Profile: validProfile()})
		if err != nil {
			t.Fatal(err)
		}
		profile := first.Snapshot.Profile
		profile.Identity.Name = "Example Site"
		second, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{
			ExpectedRevision: first.Snapshot.Revision,
			Profile:          profile,
		})
		if err != nil {
			t.Fatal(err)
		}
		if second.Changed || second.Snapshot.Revision != first.Snapshot.Revision || second.Snapshot.ETag != first.Snapshot.ETag {
			t.Fatalf("no-op result = %#v", second)
		}
	})
	t.Run("stale revision conflicts", func(t *testing.T) {
		module := factory(t, fixedClock{value: instant()})
		first, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{Profile: validProfile()})
		if err != nil {
			t.Fatal(err)
		}
		next := validProfile()
		next.Identity.Tagline = "Changed"
		if _, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{
			ExpectedRevision: first.Snapshot.Revision, Profile: next,
		}); err != nil {
			t.Fatal(err)
		}
		_, err = module.Replace(context.Background(), siteprofile.ReplaceCommand{
			ExpectedRevision: first.Snapshot.Revision, Profile: validProfile(),
		})
		var conflict *siteprofile.RevisionConflictError
		if !errors.As(err, &conflict) || conflict.Actual != 2 {
			t.Fatalf("error = %#v, want revision conflict at 2", err)
		}
	})
	t.Run("concurrent writers have one winner", func(t *testing.T) {
		module := factory(t, fixedClock{value: instant()})
		first, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{Profile: validProfile()})
		if err != nil {
			t.Fatal(err)
		}
		var wait sync.WaitGroup
		start := make(chan struct{})
		errorsOut := make(chan error, 2)
		for _, tagline := range []string{"Writer A", "Writer B"} {
			wait.Add(1)
			go func(value string) {
				defer wait.Done()
				<-start
				profile := validProfile()
				profile.Identity.Tagline = value
				_, replaceErr := module.Replace(context.Background(), siteprofile.ReplaceCommand{
					ExpectedRevision: first.Snapshot.Revision, Profile: profile,
				})
				errorsOut <- replaceErr
			}(tagline)
		}
		close(start)
		wait.Wait()
		close(errorsOut)
		successes, conflicts := 0, 0
		for replaceErr := range errorsOut {
			if replaceErr == nil {
				successes++
				continue
			}
			var conflict *siteprofile.RevisionConflictError
			if errors.As(replaceErr, &conflict) {
				conflicts++
				continue
			}
			t.Fatalf("unexpected error: %v", replaceErr)
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
		}
	})
	t.Run("invalid and unsafe input is rejected", func(t *testing.T) {
		module := factory(t, fixedClock{value: instant()})
		profile := validProfile()
		profile.Identity.Name = ""
		profile.Footer.LinkGroups[0].Links[0].Href = "javascript:alert(1)"
		profile.Footer.Social[0].URL = "/relative"
		_, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{Profile: profile})
		var validation *siteprofile.ValidationError
		if !errors.As(err, &validation) || len(validation.Diagnostics) < 3 {
			t.Fatalf("error = %#v, want validation diagnostics", err)
		}
	})
	t.Run("scheduled announcement bounds projection freshness", func(t *testing.T) {
		now := instant()
		module := factory(t, fixedClock{value: now})
		profile := validProfile()
		start := now.Add(time.Hour)
		end := now.Add(2 * time.Hour)
		profile.Announcement.StartsAt = &start
		profile.Announcement.EndsAt = &end
		if _, err := module.Replace(context.Background(), siteprofile.ReplaceCommand{Profile: profile}); err != nil {
			t.Fatal(err)
		}
		before, err := module.PublicAt(context.Background(), now)
		if err != nil {
			t.Fatal(err)
		}
		if before.Snapshot.Profile.Announcement.Enabled || before.NextChangeAt == nil || !before.NextChangeAt.Equal(start) {
			t.Fatalf("before projection = %#v", before)
		}
		active, err := module.PublicAt(context.Background(), now.Add(90*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if !active.Snapshot.Profile.Announcement.Enabled || active.NextChangeAt == nil || !active.NextChangeAt.Equal(end) {
			t.Fatalf("active projection = %#v", active)
		}
		after, err := module.PublicAt(context.Background(), now.Add(3*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if after.Snapshot.Profile.Announcement.Enabled || after.NextChangeAt != nil {
			t.Fatalf("after projection = %#v", after)
		}
	})
	t.Run("schema is stable and isolated from caller mutation", func(t *testing.T) {
		module := factory(t, fixedClock{value: instant()})
		schema := module.Schema()
		if schema.Version != 1 || schema.Digest == "" || len(schema.Sections) < 5 {
			t.Fatalf("schema = %#v", schema)
		}
		schema.Sections[0].Label = "mutated"
		if module.Schema().Sections[0].Label == "mutated" {
			t.Fatal("Schema returned shared mutable state")
		}
	})
	t.Run("archive verifies and restores through Replace", func(t *testing.T) {
		source := factory(t, fixedClock{value: instant()})
		created, err := source.Replace(context.Background(), siteprofile.ReplaceCommand{Profile: validProfile()})
		if err != nil {
			t.Fatal(err)
		}
		var archive bytes.Buffer
		manifest, err := source.Export(context.Background(), &archive)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Revision != created.Snapshot.Revision || manifest.ArchiveDigest == "" {
			t.Fatalf("manifest = %#v", manifest)
		}
		report, err := source.VerifyArchive(bytes.NewReader(archive.Bytes()))
		if err != nil || !report.Valid {
			t.Fatalf("verify report=%#v err=%v", report, err)
		}
		target := factory(t, fixedClock{value: instant().Add(time.Hour)})
		restored, err := target.Restore(context.Background(), siteprofile.RestoreCommand{}, bytes.NewReader(archive.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if restored.Result == nil || restored.Result.Snapshot.Profile.Identity.Name != "Example Site" {
			t.Fatalf("restore = %#v", restored)
		}
		tampered := bytes.Replace(archive.Bytes(), []byte("Example Site"), []byte("Tampered Site"), 1)
		tamperedReport, err := source.VerifyArchive(bytes.NewReader(tampered))
		if err != nil {
			t.Fatal(err)
		}
		if tamperedReport.Valid {
			t.Fatal("tampered archive verified")
		}
	})
}

func ValidProfile() siteprofile.Profile {
	return validProfile()
}

func validProfile() siteprofile.Profile {
	return siteprofile.Profile{
		Identity: siteprofile.Identity{Name: "  Example Site  ", Tagline: "A useful place"},
		Branding: siteprofile.Branding{Logo: &siteprofile.Visual{Kind: siteprofile.VisualIcon, Ref: "i-tabler-world", Alt: "Example"}},
		Announcement: siteprofile.Announcement{
			Enabled: true, Text: "Welcome", Tone: siteprofile.AnnouncementInfo, Dismissible: true,
			Action: &siteprofile.Link{ID: "announcement-action", Label: "Read", Href: "/news"},
		},
		Support: siteprofile.Support{Contacts: []siteprofile.Contact{
			{ID: "support-email", Kind: siteprofile.ContactEmail, Label: "Email", Value: "support@example.com"},
		}},
		Footer: siteprofile.Footer{
			Tagline: "A useful footer", Copyright: "Example",
			LinkGroups: []siteprofile.LinkGroup{
				{ID: "about", Title: "About", Links: []siteprofile.Link{{ID: "about-us", Label: "About us", Href: "/about"}}},
			},
			Social: []siteprofile.SocialLink{
				{ID: "github", Platform: "github", Label: "GitHub", URL: "https://github.com/example"},
			},
			Legal: []siteprofile.Link{{ID: "privacy", Label: "Privacy", Href: "/privacy"}},
			Compliance: siteprofile.Compliance{Records: []siteprofile.ComplianceRecord{
				{ID: "icp", Kind: "icp", Label: "ICP", Number: "Example ICP", URL: "https://beian.miit.gov.cn/"},
			}},
		},
	}
}

type fixedClock struct {
	value time.Time
}

func (c fixedClock) Now() time.Time { return c.value }

func instant() time.Time {
	return time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
}
