package siteprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

type Definition struct {
	SchemaVersion        uint64
	RequireTagline       bool
	RequireLogo          bool
	RequireFooterTagline bool
	RequireCopyright     bool
	MaxNameLength        int
	MaxTaglineLength     int
	MaxTextLength        int
	MaxURLLength         int
	MaxContacts          int
	MaxLinkGroups        int
	MaxLinksPerGroup     int
	MaxSocialLinks       int
	MaxLegalLinks        int
	MaxComplianceRecords int
	AllowedIconPrefixes  []string
}

type CompiledDefinition struct {
	value  Definition
	schema FormSchema
}

func DefaultDefinition() Definition {
	return Definition{
		SchemaVersion: 1,
		MaxNameLength: 120, MaxTaglineLength: 300, MaxTextLength: 2000, MaxURLLength: 2048,
		MaxContacts: 12, MaxLinkGroups: 12, MaxLinksPerGroup: 24, MaxSocialLinks: 24,
		MaxLegalLinks: 12, MaxComplianceRecords: 12,
		AllowedIconPrefixes: []string{"i-"},
	}
}

func CompileDefinition(in Definition) (CompiledDefinition, error) {
	defaults := DefaultDefinition()
	if in.SchemaVersion == 0 {
		in.SchemaVersion = defaults.SchemaVersion
	}
	fillPositive := func(value *int, fallback int) {
		if *value == 0 {
			*value = fallback
		}
	}
	fillPositive(&in.MaxNameLength, defaults.MaxNameLength)
	fillPositive(&in.MaxTaglineLength, defaults.MaxTaglineLength)
	fillPositive(&in.MaxTextLength, defaults.MaxTextLength)
	fillPositive(&in.MaxURLLength, defaults.MaxURLLength)
	fillPositive(&in.MaxContacts, defaults.MaxContacts)
	fillPositive(&in.MaxLinkGroups, defaults.MaxLinkGroups)
	fillPositive(&in.MaxLinksPerGroup, defaults.MaxLinksPerGroup)
	fillPositive(&in.MaxSocialLinks, defaults.MaxSocialLinks)
	fillPositive(&in.MaxLegalLinks, defaults.MaxLegalLinks)
	fillPositive(&in.MaxComplianceRecords, defaults.MaxComplianceRecords)
	if len(in.AllowedIconPrefixes) == 0 {
		in.AllowedIconPrefixes = slices.Clone(defaults.AllowedIconPrefixes)
	}
	values := []int{
		in.MaxNameLength, in.MaxTaglineLength, in.MaxTextLength, in.MaxURLLength, in.MaxContacts,
		in.MaxLinkGroups, in.MaxLinksPerGroup, in.MaxSocialLinks, in.MaxLegalLinks,
		in.MaxComplianceRecords,
	}
	for _, value := range values {
		if value < 1 {
			return CompiledDefinition{}, errors.New("siteprofile: definition limits must be positive")
		}
	}
	for _, prefix := range in.AllowedIconPrefixes {
		if prefix == "" {
			return CompiledDefinition{}, errors.New("siteprofile: icon prefix cannot be empty")
		}
	}
	compiled := CompiledDefinition{value: in}
	compiled.schema = buildFormSchema(in)
	return compiled, nil
}

func MustCompileDefinition(in Definition) CompiledDefinition {
	out, err := CompileDefinition(in)
	if err != nil {
		panic(err)
	}
	return out
}

func (d CompiledDefinition) Definition() Definition {
	out := d.value
	out.AllowedIconPrefixes = slices.Clone(out.AllowedIconPrefixes)
	return out
}

func buildFormSchema(d Definition) FormSchema {
	linkFields := []FormField{
		{Path: "id", Label: "标识", Control: ControlText, Required: true, MaxLength: 120},
		{Path: "label", Label: "名称", Control: ControlText, Required: true, MaxLength: d.MaxNameLength},
		{Path: "href", Label: "链接", Control: ControlText, Required: true, MaxLength: d.MaxURLLength},
		{Path: "icon", Label: "图标", Control: ControlText, MaxLength: 200},
	}
	sections := []FormSection{
		{ID: "identity", Label: "站点资料", Fields: []FormField{
			{Path: "identity.name", Label: "站点名称", Control: ControlText, Required: true, MaxLength: d.MaxNameLength},
			{Path: "identity.tagline", Label: "站点标语", Control: ControlTextarea, Required: d.RequireTagline, MaxLength: d.MaxTaglineLength},
			{Path: "identity.description", Label: "站点说明", Control: ControlTextarea, MaxLength: d.MaxTextLength},
		}},
		{ID: "branding", Label: "品牌资产", Fields: []FormField{
			{Path: "branding.logo", Label: "主 Logo", Control: ControlVisual, Required: d.RequireLogo},
			{Path: "branding.darkLogo", Label: "深色 Logo", Control: ControlVisual},
			{Path: "branding.favicon", Label: "站点图标", Control: ControlVisual},
		}},
		{ID: "announcement", Label: "公告", Fields: []FormField{
			{Path: "announcement.enabled", Label: "显示公告", Control: ControlToggle},
			{Path: "announcement.text", Label: "公告内容", Control: ControlTextarea, MaxLength: d.MaxTextLength},
			{Path: "announcement.tone", Label: "公告语气", Control: ControlSelect, Options: []FieldOption{
				{Value: string(AnnouncementNeutral), Label: "中性"}, {Value: string(AnnouncementInfo), Label: "信息"},
				{Value: string(AnnouncementSuccess), Label: "成功"}, {Value: string(AnnouncementWarning), Label: "提醒"},
				{Value: string(AnnouncementCritical), Label: "重要"},
			}},
			{Path: "announcement.startsAt", Label: "开始时间", Control: ControlDateTime},
			{Path: "announcement.endsAt", Label: "结束时间", Control: ControlDateTime},
		}},
		{ID: "support", Label: "支持联系", Fields: []FormField{
			{Path: "support.contacts", Label: "联系方式", Control: ControlList, MaxItems: d.MaxContacts},
		}},
		{ID: "footer", Label: "页脚", Fields: []FormField{
			{Path: "footer.tagline", Label: "页脚标语", Control: ControlTextarea, Required: d.RequireFooterTagline, MaxLength: d.MaxTaglineLength},
			{Path: "footer.copyright", Label: "版权信息", Control: ControlText, Required: d.RequireCopyright, MaxLength: d.MaxTaglineLength},
			{Path: "footer.linkGroups", Label: "链接分组", Control: ControlList, MaxItems: d.MaxLinkGroups},
			{Path: "footer.social", Label: "社交链接", Control: ControlList, MaxItems: d.MaxSocialLinks},
			{Path: "footer.legal", Label: "法律链接", Control: ControlList, MaxItems: d.MaxLegalLinks, ItemFields: linkFields},
			{Path: "footer.compliance.records", Label: "备案与合规记录", Control: ControlList, MaxItems: d.MaxComplianceRecords},
			{Path: "footer.compliance.extraText", Label: "补充法律信息", Control: ControlTextarea, MaxLength: d.MaxTextLength},
		}},
	}
	raw, err := json.Marshal(struct {
		Version  uint64
		Sections []FormSection
	}{d.SchemaVersion, sections})
	if err != nil {
		panic(fmt.Errorf("siteprofile: encode form schema: %w", err))
	}
	sum := sha256.Sum256(raw)
	return FormSchema{Version: d.SchemaVersion, Digest: Digest(hex.EncodeToString(sum[:])), Sections: sections}
}
