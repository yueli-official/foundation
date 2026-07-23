package siteprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var stableIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,119}$`)

func normalizeProfile(in Profile) Profile {
	in.Identity.Name = strings.TrimSpace(in.Identity.Name)
	in.Identity.Tagline = strings.TrimSpace(in.Identity.Tagline)
	in.Identity.Description = strings.TrimSpace(in.Identity.Description)
	in.Branding.Logo = normalizeVisual(in.Branding.Logo)
	in.Branding.DarkLogo = normalizeVisual(in.Branding.DarkLogo)
	in.Branding.Favicon = normalizeVisual(in.Branding.Favicon)
	in.Announcement.Text = strings.TrimSpace(in.Announcement.Text)
	in.Announcement.Action = normalizeLinkPointer(in.Announcement.Action)
	if in.Announcement.Tone == "" {
		in.Announcement.Tone = AnnouncementNeutral
	}
	if in.Announcement.StartsAt != nil {
		value := in.Announcement.StartsAt.UTC()
		in.Announcement.StartsAt = &value
	}
	if in.Announcement.EndsAt != nil {
		value := in.Announcement.EndsAt.UTC()
		in.Announcement.EndsAt = &value
	}
	if in.Support.Contacts == nil {
		in.Support.Contacts = []Contact{}
	}
	for index := range in.Support.Contacts {
		item := &in.Support.Contacts[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Label = strings.TrimSpace(item.Label)
		item.Value = strings.TrimSpace(item.Value)
		item.Icon = strings.TrimSpace(item.Icon)
	}
	in.Footer.Tagline = strings.TrimSpace(in.Footer.Tagline)
	in.Footer.Copyright = strings.TrimSpace(in.Footer.Copyright)
	if in.Footer.LinkGroups == nil {
		in.Footer.LinkGroups = []LinkGroup{}
	}
	for groupIndex := range in.Footer.LinkGroups {
		group := &in.Footer.LinkGroups[groupIndex]
		group.ID = strings.TrimSpace(group.ID)
		group.Title = strings.TrimSpace(group.Title)
		if group.Links == nil {
			group.Links = []Link{}
		}
		for linkIndex := range group.Links {
			group.Links[linkIndex] = normalizeLink(group.Links[linkIndex])
		}
	}
	if in.Footer.Social == nil {
		in.Footer.Social = []SocialLink{}
	}
	for index := range in.Footer.Social {
		item := &in.Footer.Social[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Platform = strings.TrimSpace(item.Platform)
		item.Label = strings.TrimSpace(item.Label)
		item.URL = strings.TrimSpace(item.URL)
		item.Icon = strings.TrimSpace(item.Icon)
	}
	if in.Footer.Legal == nil {
		in.Footer.Legal = []Link{}
	}
	for index := range in.Footer.Legal {
		in.Footer.Legal[index] = normalizeLink(in.Footer.Legal[index])
	}
	if in.Footer.Compliance.Records == nil {
		in.Footer.Compliance.Records = []ComplianceRecord{}
	}
	for index := range in.Footer.Compliance.Records {
		item := &in.Footer.Compliance.Records[index]
		item.ID = strings.TrimSpace(item.ID)
		item.Kind = strings.TrimSpace(item.Kind)
		item.Label = strings.TrimSpace(item.Label)
		item.Number = strings.TrimSpace(item.Number)
		item.URL = strings.TrimSpace(item.URL)
	}
	in.Footer.Compliance.ExtraText = strings.TrimSpace(in.Footer.Compliance.ExtraText)
	return in
}

func normalizeVisual(in *Visual) *Visual {
	if in == nil {
		return nil
	}
	out := *in
	out.Ref = strings.TrimSpace(out.Ref)
	out.Alt = strings.TrimSpace(out.Alt)
	if out.Kind == "" && out.Ref == "" && out.Alt == "" {
		return nil
	}
	return &out
}

func normalizeLinkPointer(in *Link) *Link {
	if in == nil {
		return nil
	}
	out := normalizeLink(*in)
	if out.ID == "" && out.Label == "" && out.Href == "" && out.Icon == "" {
		return nil
	}
	return &out
}

func normalizeLink(in Link) Link {
	in.ID = strings.TrimSpace(in.ID)
	in.Label = strings.TrimSpace(in.Label)
	in.Href = strings.TrimSpace(in.Href)
	in.Icon = strings.TrimSpace(in.Icon)
	return in
}

func encodeProfile(profile Profile) ([]byte, Digest, error) {
	raw, err := json.Marshal(profile)
	if err != nil {
		return nil, "", fmt.Errorf("siteprofile: encode profile: %w", err)
	}
	sum := sha256.Sum256(raw)
	return raw, Digest(hex.EncodeToString(sum[:])), nil
}

func decodeProfile(raw []byte) (Profile, error) {
	var profile Profile
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return Profile{}, fmt.Errorf("%w: decode document: %v", ErrCorruptState, err)
	}
	return profile, nil
}

func validateProfile(profile Profile, definition Definition) []Diagnostic {
	var diagnostics []Diagnostic
	requireText := func(path, value string, required bool, max int) {
		if required && value == "" {
			diagnostics = append(diagnostics, Diagnostic{Code: "required", Path: path, Message: path + " is required"})
		}
		if utf8.RuneCountInString(value) > max {
			diagnostics = append(diagnostics, Diagnostic{Code: "too_long", Path: path, Message: path + " is too long"})
		}
	}
	requireText("identity.name", profile.Identity.Name, true, definition.MaxNameLength)
	requireText("identity.tagline", profile.Identity.Tagline, definition.RequireTagline, definition.MaxTaglineLength)
	requireText("identity.description", profile.Identity.Description, false, definition.MaxTextLength)
	requireText("footer.tagline", profile.Footer.Tagline, definition.RequireFooterTagline, definition.MaxTaglineLength)
	requireText("footer.copyright", profile.Footer.Copyright, definition.RequireCopyright, definition.MaxTaglineLength)
	validateVisual := func(path string, visual *Visual, required bool) {
		if visual == nil {
			if required {
				diagnostics = append(diagnostics, Diagnostic{Code: "required", Path: path, Message: path + " is required"})
			}
			return
		}
		if visual.Kind != VisualIcon && visual.Kind != VisualAsset {
			diagnostics = append(diagnostics, Diagnostic{Code: "visual_kind", Path: path + ".kind", Message: "visual kind must be icon or asset"})
		}
		if visual.Ref == "" || utf8.RuneCountInString(visual.Ref) > 200 {
			diagnostics = append(diagnostics, Diagnostic{Code: "visual_ref", Path: path + ".ref", Message: "visual reference is invalid"})
		}
		if visual.Kind == VisualIcon && !hasPrefix(visual.Ref, definition.AllowedIconPrefixes) {
			diagnostics = append(diagnostics, Diagnostic{Code: "icon_not_allowed", Path: path + ".ref", Message: "icon reference is not allowed"})
		}
		if utf8.RuneCountInString(visual.Alt) > definition.MaxNameLength {
			diagnostics = append(diagnostics, Diagnostic{Code: "too_long", Path: path + ".alt", Message: "visual alt text is too long"})
		}
	}
	validateVisual("branding.logo", profile.Branding.Logo, definition.RequireLogo)
	validateVisual("branding.darkLogo", profile.Branding.DarkLogo, false)
	validateVisual("branding.favicon", profile.Branding.Favicon, false)

	tones := []AnnouncementTone{
		AnnouncementNeutral, AnnouncementInfo, AnnouncementSuccess, AnnouncementWarning, AnnouncementCritical,
	}
	if !slices.Contains(tones, profile.Announcement.Tone) {
		diagnostics = append(diagnostics, Diagnostic{Code: "announcement_tone", Path: "announcement.tone", Message: "announcement tone is invalid"})
	}
	requireText("announcement.text", profile.Announcement.Text, profile.Announcement.Enabled, definition.MaxTextLength)
	if profile.Announcement.StartsAt != nil && profile.Announcement.EndsAt != nil &&
		!profile.Announcement.StartsAt.Before(*profile.Announcement.EndsAt) {
		diagnostics = append(diagnostics, Diagnostic{Code: "announcement_window", Path: "announcement.endsAt", Message: "announcement end must be after start"})
	}
	if profile.Announcement.Action != nil {
		diagnostics = append(diagnostics, validateLink("announcement.action", *profile.Announcement.Action, definition)...)
	}

	if len(profile.Support.Contacts) > definition.MaxContacts {
		diagnostics = append(diagnostics, Diagnostic{Code: "too_many_items", Path: "support.contacts", Message: "too many support contacts"})
	}
	contactIDs := map[string]struct{}{}
	for index, contact := range profile.Support.Contacts {
		path := fmt.Sprintf("support.contacts[%d]", index)
		diagnostics = append(diagnostics, validateStableID(path+".id", contact.ID, contactIDs)...)
		requireText(path+".label", contact.Label, true, definition.MaxNameLength)
		requireText(path+".value", contact.Value, true, definition.MaxURLLength)
		switch contact.Kind {
		case ContactEmail:
			parsed, err := mail.ParseAddress(contact.Value)
			if err != nil || parsed.Address != contact.Value {
				diagnostics = append(diagnostics, Diagnostic{Code: "contact_email", Path: path + ".value", Message: "support email is invalid"})
			}
		case ContactPhone, ContactText:
		case ContactLink:
			if !validHref(contact.Value, definition.MaxURLLength) {
				diagnostics = append(diagnostics, Diagnostic{Code: "link_invalid", Path: path + ".value", Message: "support link is invalid"})
			}
		default:
			diagnostics = append(diagnostics, Diagnostic{Code: "contact_kind", Path: path + ".kind", Message: "support contact kind is invalid"})
		}
	}

	if len(profile.Footer.LinkGroups) > definition.MaxLinkGroups {
		diagnostics = append(diagnostics, Diagnostic{Code: "too_many_items", Path: "footer.linkGroups", Message: "too many footer link groups"})
	}
	groupIDs := map[string]struct{}{}
	for groupIndex, group := range profile.Footer.LinkGroups {
		path := fmt.Sprintf("footer.linkGroups[%d]", groupIndex)
		diagnostics = append(diagnostics, validateStableID(path+".id", group.ID, groupIDs)...)
		requireText(path+".title", group.Title, true, definition.MaxNameLength)
		if len(group.Links) > definition.MaxLinksPerGroup {
			diagnostics = append(diagnostics, Diagnostic{Code: "too_many_items", Path: path + ".links", Message: "too many links in footer group"})
		}
		linkIDs := map[string]struct{}{}
		for linkIndex, link := range group.Links {
			linkPath := fmt.Sprintf("%s.links[%d]", path, linkIndex)
			diagnostics = append(diagnostics, validateLinkWithIDs(linkPath, link, definition, linkIDs)...)
		}
	}
	if len(profile.Footer.Social) > definition.MaxSocialLinks {
		diagnostics = append(diagnostics, Diagnostic{Code: "too_many_items", Path: "footer.social", Message: "too many social links"})
	}
	socialIDs := map[string]struct{}{}
	for index, social := range profile.Footer.Social {
		path := fmt.Sprintf("footer.social[%d]", index)
		diagnostics = append(diagnostics, validateStableID(path+".id", social.ID, socialIDs)...)
		requireText(path+".platform", social.Platform, true, definition.MaxNameLength)
		requireText(path+".label", social.Label, false, definition.MaxNameLength)
		if !validAbsoluteHTTPURL(social.URL, definition.MaxURLLength) {
			diagnostics = append(diagnostics, Diagnostic{Code: "url_invalid", Path: path + ".url", Message: "social URL must be absolute HTTP(S)"})
		}
	}
	if len(profile.Footer.Legal) > definition.MaxLegalLinks {
		diagnostics = append(diagnostics, Diagnostic{Code: "too_many_items", Path: "footer.legal", Message: "too many legal links"})
	}
	legalIDs := map[string]struct{}{}
	for index, link := range profile.Footer.Legal {
		path := fmt.Sprintf("footer.legal[%d]", index)
		diagnostics = append(diagnostics, validateLinkWithIDs(path, link, definition, legalIDs)...)
	}
	if len(profile.Footer.Compliance.Records) > definition.MaxComplianceRecords {
		diagnostics = append(diagnostics, Diagnostic{Code: "too_many_items", Path: "footer.compliance.records", Message: "too many compliance records"})
	}
	recordIDs := map[string]struct{}{}
	for index, record := range profile.Footer.Compliance.Records {
		path := fmt.Sprintf("footer.compliance.records[%d]", index)
		diagnostics = append(diagnostics, validateStableID(path+".id", record.ID, recordIDs)...)
		requireText(path+".kind", record.Kind, true, definition.MaxNameLength)
		requireText(path+".label", record.Label, true, definition.MaxNameLength)
		requireText(path+".number", record.Number, true, definition.MaxTaglineLength)
		if record.URL != "" && !validAbsoluteHTTPURL(record.URL, definition.MaxURLLength) {
			diagnostics = append(diagnostics, Diagnostic{Code: "url_invalid", Path: path + ".url", Message: "compliance URL must be absolute HTTP(S)"})
		}
	}
	requireText("footer.compliance.extraText", profile.Footer.Compliance.ExtraText, false, definition.MaxTextLength)
	return diagnostics
}

func validateLinkWithIDs(path string, link Link, definition Definition, seen map[string]struct{}) []Diagnostic {
	diagnostics := validateStableID(path+".id", link.ID, seen)
	return append(diagnostics, validateLink(path, link, definition)...)
}

func validateLink(path string, link Link, definition Definition) []Diagnostic {
	var diagnostics []Diagnostic
	if link.Label == "" {
		diagnostics = append(diagnostics, Diagnostic{Code: "required", Path: path + ".label", Message: "link label is required"})
	}
	if utf8.RuneCountInString(link.Label) > definition.MaxNameLength {
		diagnostics = append(diagnostics, Diagnostic{Code: "too_long", Path: path + ".label", Message: "link label is too long"})
	}
	if !validHref(link.Href, definition.MaxURLLength) {
		diagnostics = append(diagnostics, Diagnostic{Code: "link_invalid", Path: path + ".href", Message: "link target is invalid"})
	}
	return diagnostics
}

func validateStableID(path, id string, seen map[string]struct{}) []Diagnostic {
	if !stableIDPattern.MatchString(id) {
		return []Diagnostic{{Code: "id_invalid", Path: path, Message: "stable ID is invalid"}}
	}
	if _, exists := seen[id]; exists {
		return []Diagnostic{{Code: "id_duplicate", Path: path, Message: "stable ID is duplicated"}}
	}
	seen[id] = struct{}{}
	return nil
}

func hasPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func validHref(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength || strings.ContainsAny(value, "\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.IsAbs() {
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			return parsed.Host != ""
		case "mailto", "tel":
			return parsed.Opaque != "" || parsed.Path != ""
		default:
			return false
		}
	}
	return strings.HasPrefix(parsed.Path, "/") && !strings.HasPrefix(parsed.Path, "//")
}

func validAbsoluteHTTPURL(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength || strings.ContainsAny(value, "\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
