package authorization_test

import (
	"slices"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestCompileBuildsAValidatedCatalog(t *testing.T) {
	catalog, err := authorization.Compile(validDefinition())
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if got, want := catalog.Consumer(), authorization.ConsumerKey("docs"); got != want {
		t.Fatalf("Consumer() = %q, want %q", got, want)
	}
	if got, want := catalog.ProtectedRole(), authorization.RoleKey("administrator"); got != want {
		t.Fatalf("ProtectedRole() = %q, want %q", got, want)
	}
	if !catalog.HasCapability("docs.document.publish") {
		t.Fatal("catalog does not contain docs.document.publish")
	}
	if !catalog.AllowsScopeChild("site", "collection") || catalog.AllowsScopeChild("site", "document") {
		t.Fatal("scope schema did not preserve its declared edges")
	}
	if catalog.Digest() == "" {
		t.Fatal("Digest() is empty")
	}
}

func TestCompileRejectsDisconnectedScopeType(t *testing.T) {
	definition := validDefinition()
	definition.Scopes.Types = append(definition.Scopes.Types, authorization.ScopeTypeDefinition{Key: "orphan"})

	_, err := authorization.Compile(definition)
	if !authorization.IsInvalidDefinition(err) {
		t.Fatalf("Compile() error = %v, want invalid definition", err)
	}
}

func TestCompileRejectsUnknownAllowedScope(t *testing.T) {
	definition := validDefinition()
	definition.Capabilities[0].AllowedScopes = []authorization.ScopeType{"missing"}

	_, err := authorization.Compile(definition)
	if !authorization.IsInvalidDefinition(err) {
		t.Fatalf("Compile() error = %v, want invalid definition", err)
	}
}

func TestCatalogDigestDoesNotDependOnDeclarationOrder(t *testing.T) {
	first := validDefinition()
	second := validDefinition()
	slices.Reverse(second.Scopes.Types)
	slices.Reverse(second.Roles)
	slices.Reverse(second.AccessLayers)
	slices.Reverse(second.Roles[0].Capabilities)

	firstCatalog := authorization.MustCompile(first)
	secondCatalog := authorization.MustCompile(second)
	if firstCatalog.Digest() != secondCatalog.Digest() {
		t.Fatalf("digest changed with declaration order:\n%s\n%s", firstCatalog.Digest(), secondCatalog.Digest())
	}
}

func TestCompileRejectsProtectedRoleWithoutAuthorizationManage(t *testing.T) {
	definition := validDefinition()
	definition.Roles[0].Capabilities = []authorization.CapabilityKey{"docs.document.publish"}

	_, err := authorization.Compile(definition)
	if !authorization.IsInvalidDefinition(err) {
		t.Fatalf("Compile() error = %v, want invalid definition", err)
	}
}

func validDefinition() authorization.Definition {
	return authorization.Definition{
		Consumer: "docs",
		Version:  1,
		Capabilities: []authorization.CapabilityDefinition{
			{Key: "docs.document.publish", Version: 1, Binding: authorization.BindingNormal},
		},
		Scopes: authorization.ScopeSchema{
			Types: []authorization.ScopeTypeDefinition{
				{Key: "site", Root: true, Children: []authorization.ScopeType{"collection"}},
				{Key: "collection", Children: []authorization.ScopeType{"document"}},
				{Key: "document"},
			},
		},
		AccessLayers: []authorization.AccessLayerDefinition{
			{Key: authorization.AccessLayerVisitor},
			{
				Key:          authorization.AccessLayerAuthenticated,
				Capabilities: []authorization.CapabilityKey{authorization.CapabilityApplicationCreate},
			},
		},
		Roles: []authorization.RoleDefinition{
			{
				Key:          "administrator",
				DisplayName:  "管理员",
				Protected:    true,
				Capabilities: []authorization.CapabilityKey{authorization.CapabilityManage, "docs.document.publish"},
			},
			{
				Key:          "author",
				DisplayName:  "作者",
				Capabilities: []authorization.CapabilityKey{"docs.document.publish"},
				Assignment: authorization.AssignmentPolicy{
					Sources: []authorization.GrantSource{
						authorization.GrantSourceApplication,
						authorization.GrantSourceDirect,
						authorization.GrantSourceGroup,
					},
				},
			},
		},
	}
}
