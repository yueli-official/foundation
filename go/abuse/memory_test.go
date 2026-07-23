package abuse_test

import (
	"testing"
	"time"

	"github.com/yueli-official/foundation/go/abuse"
	"github.com/yueli-official/foundation/go/abuse/abusetest"
)

func TestMemoryConformance(t *testing.T) {
	abusetest.Run(t, func(
		t *testing.T,
		catalog *abuse.Catalog,
		clock func() time.Time,
		verifiers map[abuse.ChallengeKind]abuse.ChallengeVerifier,
	) abuse.Module {
		t.Helper()
		module, err := abuse.NewMemory(catalog, abuse.MemoryOptions{
			Clock: clock, Secret: []byte("01234567890123456789012345678901"),
			Verifiers: verifiers,
		})
		if err != nil {
			t.Fatal(err)
		}
		return module
	})
}
