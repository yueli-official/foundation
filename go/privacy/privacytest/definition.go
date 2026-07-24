package privacytest

import (
	"time"

	"github.com/yueli-official/foundation/go/privacy"
)

var (
	NewsletterPurpose = privacy.PurposeRef{Key: "blog.newsletter", Revision: 1}
	SecurityPurpose   = privacy.PurposeRef{Key: "identity.security", Revision: 1}
	NewsletterNotice  = privacy.NoticeRef{Key: "blog.newsletter", Revision: 1}
	CommentRetention  = privacy.RetentionRuleRef{Key: "blog.comment_network", Revision: 1}
)

func Definition(now time.Time) privacy.Definition {
	blogOwner := privacy.OwnerDefinition{
		Ref:          privacy.OwnerRef{Key: "blog", Revision: 1},
		SubjectKinds: []privacy.SubjectKind{"user", "address"},
		Datasets: []privacy.DatasetDefinition{
			{
				Key: "blog.comments", Categories: []privacy.DataCategoryKey{"public_content", "network_security"},
				Operations:     []privacy.RightsOperation{privacy.RightAccess, privacy.RightErasure},
				RetentionRules: []privacy.RetentionRuleRef{CommentRetention},
			},
			{
				Key: "blog.newsletter", Categories: []privacy.DataCategoryKey{"marketing_contact"},
				Operations: []privacy.RightsOperation{privacy.RightAccess, privacy.RightErasure},
			},
		},
	}
	identityOwner := privacy.OwnerDefinition{
		Ref:                 privacy.OwnerRef{Key: "identity", Revision: 1},
		SubjectKinds:        []privacy.SubjectKind{"user"},
		FinalizeAfterOwners: true,
		Datasets: []privacy.DatasetDefinition{
			{
				Key: "identity.account", Categories: []privacy.DataCategoryKey{"account_contact"},
				Operations: []privacy.RightsOperation{privacy.RightAccess, privacy.RightErasure},
			},
		},
	}
	return privacy.Definition{
		Version: privacy.DefinitionVersion, Consumer: "privacy-conformance",
		SubjectKinds: []privacy.SubjectKindDefinition{
			{Key: "user", MaxRefBytes: 200},
			{Key: "address", MaxRefBytes: 200},
		},
		DataCategories: []privacy.DataCategoryDefinition{
			{Key: "account_contact"}, {Key: "marketing_contact"},
			{Key: "network_security"}, {Key: "public_content"},
		},
		Signals: []privacy.SignalDefinition{{Key: "gpc", MaxEvidenceAge: 24 * time.Hour}},
		Notices: []privacy.NoticeDefinition{{
			Ref: NewsletterNotice, ContentDigest: "sha256:notice-v1",
			Purposes: []privacy.PurposeRef{NewsletterPurpose}, PublishedAt: now.Add(-time.Hour),
		}},
		Purposes: []privacy.PurposeDefinition{
			{
				Ref: NewsletterPurpose, Basis: privacy.BasisConsent,
				Categories:  []privacy.DataCategoryKey{"marketing_contact"},
				Notices:     []privacy.NoticeRef{NewsletterNotice},
				SignalRules: []privacy.SignalRule{{Signal: "gpc", Effect: privacy.SignalDeny}},
			},
			{
				Ref: SecurityPurpose, Basis: privacy.BasisLegalObligation,
				Categories: []privacy.DataCategoryKey{"network_security"},
			},
		},
		ActivePurposes: []privacy.ActivePurpose{
			{Key: NewsletterPurpose.Key, Ref: NewsletterPurpose},
			{Key: SecurityPurpose.Key, Ref: SecurityPurpose},
		},
		RetentionRules: []privacy.RetentionRuleDefinition{{
			Ref: CommentRetention, Categories: []privacy.DataCategoryKey{"network_security"},
			Trigger: "record_created", ReviewAfter: privacy.CalendarPeriod{Days: 30},
			DefaultReviewOutcome: privacy.ReviewOwnerDecision,
		}},
		Owner: &blogOwner,
		Coordination: &privacy.CoordinationDefinition{
			Owners: []privacy.OwnerDefinition{blogOwner, identityOwner},
			RightsPolicies: []privacy.RightsPolicy{
				{Operation: privacy.RightAccess, RespondWithin: privacy.CalendarPeriod{Days: 30}, VerificationMaxAge: 24 * time.Hour},
				{Operation: privacy.RightErasure, RespondWithin: privacy.CalendarPeriod{Days: 30}, VerificationMaxAge: 24 * time.Hour},
			},
		},
	}
}
