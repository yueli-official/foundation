package authorization_test

import (
	"context"
	"testing"

	"github.com/yueli-official/foundation/go/authorization"
	"github.com/yueli-official/foundation/go/authorization/authorizationtest"
)

func TestMemoryAdapterConformance(t *testing.T) {
	authorizationtest.Run(t, func(_ context.Context, setup authorizationtest.Setup) (authorizationtest.Adapter, func(), error) {
		catalog, err := authorization.Compile(setup.Definition)
		if err != nil {
			return nil, nil, err
		}
		module, err := authorization.NewMemory(catalog, authorization.MemoryOptions{
			RootScopeID:       setup.RootScopeID,
			ProtectedSubjects: setup.ProtectedSubjects,
		})
		return module, nil, err
	})
}
