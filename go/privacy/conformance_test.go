package privacy_test

import (
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/privacy"
	"github.com/yueli-official/foundation/go/privacy/privacytest"
)

func TestMemoryRuntimeConformance(t *testing.T) {
	privacytest.RunRuntime(t, func(t *testing.T, catalog *privacy.Catalog, clock func() time.Time) privacy.Runtime {
		t.Helper()
		runtime, err := privacy.NewMemory(catalog, privacy.MemoryOptions{Clock: clock})
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	})
}

func TestMemoryCoordinatorConformance(t *testing.T) {
	privacytest.RunCoordinator(t, func(
		t *testing.T,
		catalog *privacy.Catalog,
		clock func() time.Time,
		router privacy.OwnerRouter,
	) privacy.Coordinator {
		t.Helper()
		coordinator, err := privacy.NewMemoryCoordinator(catalog, privacy.MemoryCoordinatorOptions{Clock: clock, Router: router})
		if err != nil {
			t.Fatal(err)
		}
		return coordinator
	})
}
