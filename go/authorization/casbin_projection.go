package authorization

import (
	"fmt"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
)

const casbinModel = `
[request_definition]
r = sub, role, dom, cap

[policy_definition]
p = role, cap

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, r.role, r.dom) && r.role == p.role && r.cap == p.cap
`

type casbinProjection struct {
	revision uint64
	enforcer *casbin.Enforcer
}

// RebuildCasbinProjection replaces the derived execution snapshot from domain
// truth. It does not persist or mutate Role, Grant, Scope or Policy state.
func (module *Memory) RebuildCasbinProjection() error {
	module.mu.Lock()
	defer module.mu.Unlock()
	projection, err := module.buildCasbinProjectionLocked()
	if err != nil {
		module.casbin = nil
		return &Error{
			Kind: ErrorUnavailable, Field: "execution_projection",
			Message: "could not rebuild Casbin projection", err: err,
		}
	}
	module.casbin = projection
	return nil
}

func (module *Memory) buildCasbinProjectionLocked() (*casbinProjection, error) {
	casbinDefinition, err := model.NewModelFromString(casbinModel)
	if err != nil {
		return nil, fmt.Errorf("compile model: %w", err)
	}
	enforcer, err := casbin.NewEnforcer(casbinDefinition)
	if err != nil {
		return nil, fmt.Errorf("create enforcer: %w", err)
	}
	policy := module.policies[module.activePolicy]
	for _, role := range policy.roles {
		if role.Status != RoleActive {
			continue
		}
		capabilities := role.Capabilities
		if role.Protected {
			capabilities = make([]CapabilityKey, 0, len(module.catalog.capabilities))
			for capability := range module.catalog.capabilities {
				capabilities = append(capabilities, capability)
			}
		}
		for _, capability := range capabilities {
			if _, err := enforcer.AddPolicy(string(role.ID), string(capability)); err != nil {
				return nil, fmt.Errorf("add role capability: %w", err)
			}
		}
	}
	for _, grant := range module.grants {
		role, exists := policy.roles[grant.Role]
		if !exists || role.Status != RoleActive || role.ID != grant.RoleID {
			continue
		}
		var subjects []SubjectRef
		if grant.Target.Kind == SubjectGroup {
			for _, member := range module.groupMembers[GroupID(grant.Target.ID)] {
				subjects = append(subjects, member)
			}
		} else {
			subjects = append(subjects, grant.Target)
		}
		for scopeID := range module.scopes {
			if !module.scopeContainsLocked(grant.ScopeID, scopeID) {
				continue
			}
			for _, subject := range subjects {
				if _, err := enforcer.AddGroupingPolicy(
					subjectKey(subject),
					string(role.ID),
					string(scopeID),
				); err != nil {
					return nil, fmt.Errorf("add role membership: %w", err)
				}
			}
		}
	}
	return &casbinProjection{revision: module.activePolicy, enforcer: enforcer}, nil
}

func (projection *casbinProjection) allows(
	subject SubjectRef,
	roleID RoleID,
	scopeID ScopeID,
	capability CapabilityKey,
) (bool, error) {
	if projection == nil || projection.enforcer == nil {
		return false, fmt.Errorf("projection is unavailable")
	}
	return projection.enforcer.Enforce(
		subjectKey(subject),
		string(roleID),
		string(scopeID),
		string(capability),
	)
}
