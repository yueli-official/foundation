package adminapi_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/yueli-official/foundation/go/classification"
	"github.com/yueli-official/foundation/go/classification/adminapi"
)

func TestCategoryJSONContract(t *testing.T) {
	position := 7
	value := adminapi.Category{
		ID:                "games",
		ParentID:          "software",
		Slug:              "games",
		Name:              "游戏",
		Description:       "游戏相关作品",
		Status:            adminapi.StatusActive,
		Revision:          9,
		EditorialPosition: &position,
		ReplacementID:     "games-next",
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"id":"games","parentId":"software","slug":"games","name":"游戏","description":"游戏相关作品","status":"active","revision":9,"editorialPosition":7,"replacementId":"games-next"}`
	if string(raw) != want {
		t.Fatalf("category JSON = %s", raw)
	}
}

func TestFacetValueDomainRoundTripPreservesOwnerParentAndPosition(t *testing.T) {
	position := 3
	domain := classification.FacetValue{
		ID:                "windows",
		FacetID:           "platform",
		ParentID:          "desktop",
		Slug:              "windows",
		Name:              "Windows",
		Status:            classification.StatusInactive,
		EditorialPosition: &position,
	}
	wire := adminapi.FromFacetValue(domain, 12)
	if wire.Revision != 12 || wire.FacetID != "platform" || wire.ParentID != "desktop" {
		t.Fatalf("wire = %#v", wire)
	}
	if got := wire.Domain(); !reflect.DeepEqual(got, domain) {
		t.Fatalf("round trip = %#v, want %#v", got, domain)
	}
	*wire.EditorialPosition = 99
	if *domain.EditorialPosition != 3 {
		t.Fatal("wire conversion must not alias the domain position pointer")
	}
}

func TestPolicyRoundTripPreservesCanonicalFacetAssignmentPolicy(t *testing.T) {
	domain := classification.PolicyProfile{
		Key:            "video.default",
		SchemaVersion:  1,
		PolicyRevision: 8,
		Category: classification.CategoryPolicy{
			MinAssignments: 1,
			MaxAssignments: 3,
			RequirePrimary: true,
			LeafOnly:       true,
			MaxDepth:       4,
		},
		Facets: []classification.FacetAssignmentPolicy{{
			FacetID: "platform", MinValues: 1, MaxValues: 2, LeafOnly: true, MaxDepth: 3,
		}},
		Tags: classification.TagAdmissionPolicy{
			Unknown: classification.UnknownTagPropose, MinAssignments: 0, MaxAssignments: 12,
		},
		Discovery: classification.DiscoveryPolicy{DefaultSort: classification.CandidateSortEditorial},
	}
	wire := adminapi.FromPolicyProfile(domain)
	if wire.Revision != 8 || len(wire.Facets) != 1 {
		t.Fatalf("wire policy = %#v", wire)
	}
	if got := wire.Domain(); !reflect.DeepEqual(got, domain) {
		t.Fatalf("round trip = %#v, want %#v", got, domain)
	}
}

func TestNewListAndCatalogSnapshotEncodeEmptyCollectionsAsArrays(t *testing.T) {
	listRaw, err := json.Marshal(adminapi.NewList[adminapi.Tag](nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(listRaw) != `{"items":[]}` {
		t.Fatalf("list JSON = %s", listRaw)
	}

	snapshotRaw, err := json.Marshal(adminapi.NewCatalogSnapshot("blog", 4))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schemaVersion":1`, `"categories":[]`, `"facets":[]`, `"facetValues":[]`, `"tags":[]`, `"policies":[]`} {
		if !strings.Contains(string(snapshotRaw), want) {
			t.Fatalf("snapshot JSON %s missing %s", snapshotRaw, want)
		}
	}
}

func TestCanonicalResourcesDoNotAbsorbProductBindingOrPresentationFields(t *testing.T) {
	for _, value := range []any{adminapi.Category{}, adminapi.Facet{}, adminapi.FacetValue{}, adminapi.Tag{}} {
		typeOf := reflect.TypeOf(value)
		for _, forbidden := range []string{"Icon", "ImageMediaKey", "ContentCount", "AuthorVisible", "DiscoveryVisible", "AllowedValueIDs", "Source"} {
			if _, ok := typeOf.FieldByName(forbidden); ok {
				t.Fatalf("%s unexpectedly owns product field %s", typeOf.Name(), forbidden)
			}
		}
	}
}
