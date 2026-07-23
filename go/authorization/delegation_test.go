package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
)

func TestScopedManagerCanDelegateOnlyInsideItsScope(t *testing.T) {
	definition := validDefinition()
	definition.Capabilities[0].Delegable = true
	definition.Roles = append(definition.Roles, authorization.RoleDefinition{
		Key: "collection_manager", DisplayName: "文档集管理员",
		Capabilities: []authorization.CapabilityKey{
			authorization.CapabilityManage,
			"docs.document.publish",
		},
		Assignment: authorization.AssignmentPolicy{
			Sources: []authorization.GrantSource{authorization.GrantSourceDirect},
		},
	})
	ctx := context.Background()
	admin := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "admin"}
	manager := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "manager"}
	author := authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "author"}
	module, err := authorization.NewMemory(authorization.MustCompile(definition), authorization.MemoryOptions{
		RootScopeID: "docs", ProtectedSubjects: []authorization.SubjectRef{admin},
	})
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
		Actor: admin, ID: "handbook", Type: "collection", ParentID: "docs",
	}); err != nil {
		t.Fatalf("CreateScope() collection error = %v", err)
	}
	if _, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: admin, Target: manager, Role: "collection_manager", ScopeID: "handbook",
		Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() manager error = %v", err)
	}
	if _, err := module.CreateScope(ctx, authorization.CreateScopeCommand{
		Actor: manager, ID: "intro", Type: "document", ParentID: "handbook",
	}); err != nil {
		t.Fatalf("CreateScope() delegated error = %v", err)
	}
	if _, err := module.Grant(ctx, authorization.GrantCommand{
		Actor: manager, Target: author, Role: "author", ScopeID: "handbook",
		Source: authorization.GrantSourceDirect,
	}); err != nil {
		t.Fatalf("Grant() delegated author error = %v", err)
	}

	_, err = module.Grant(ctx, authorization.GrantCommand{
		Actor: manager, Target: authorization.SubjectRef{Kind: authorization.SubjectUser, ID: "outside"},
		Role: "author", ScopeID: "docs", Source: authorization.GrantSourceDirect,
	})
	if !authorization.Is(err, authorization.ErrorDenied) {
		t.Fatalf("Grant() outside scope error = %v, want denied", err)
	}
}
