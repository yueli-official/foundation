package siteprofile_test

import (
	"testing"

	"github.com/yueli-official/foundation/go/siteprofile"
	"github.com/yueli-official/foundation/go/siteprofile/siteprofiletest"
)

func TestMemoryConformance(t *testing.T) {
	definition := siteprofile.MustCompileDefinition(siteprofile.DefaultDefinition())
	siteprofiletest.Run(t, func(t *testing.T, clock siteprofile.Clock) siteprofile.Module {
		t.Helper()
		return siteprofile.NewMemory(definition, clock)
	})
}
