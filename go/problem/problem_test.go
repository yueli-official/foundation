package problem_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yueli-official/foundation/go/problem"
)

func TestCanonicalFixtures(t *testing.T) {
	t.Parallel()
	root := filepath.Join("testdata", "fixtures")

	valid, err := filepath.Glob(filepath.Join(root, "valid", "*.json"))
	if err != nil || len(valid) == 0 {
		t.Fatalf("valid fixtures: files=%d err=%v", len(valid), err)
	}
	for _, path := range valid {
		path := path
		t.Run("valid/"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			value, err := problem.Decode(data)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := problem.Decode(encoded); err != nil {
				t.Fatalf("round-trip Decode() error = %v", err)
			}
		})
	}

	invalid, err := filepath.Glob(filepath.Join(root, "invalid", "*.json"))
	if err != nil || len(invalid) == 0 {
		t.Fatalf("invalid fixtures: files=%d err=%v", len(invalid), err)
	}
	for _, path := range invalid {
		path := path
		t.Run("invalid/"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := problem.Decode(data); err == nil {
				t.Fatal("Decode() unexpectedly accepted invalid fixture")
			}
		})
	}
}

func TestPackagedContractSnapshotMatchesCanonical(t *testing.T) {
	t.Parallel()
	canonicalRoot := filepath.Join("..", "..", "contracts", "http-problem")
	if _, err := os.Stat(canonicalRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skip("repository-level canonical contract is not present in the published Go module")
		}
		t.Fatal(err)
	}

	err := filepath.WalkDir("testdata", func(snapshotPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel("testdata", snapshotPath)
		if err != nil {
			return err
		}
		snapshot, err := os.ReadFile(snapshotPath)
		if err != nil {
			return err
		}
		canonical, err := os.ReadFile(filepath.Join(canonicalRoot, relative))
		if err != nil {
			return err
		}
		if !bytes.Equal(snapshot, canonical) {
			return fmt.Errorf("Go module contract snapshot is stale: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestKindAndConstructionAreExplicit(t *testing.T) {
	t.Parallel()
	kind := problem.MustKind("common.rate_limited", 429)
	value, err := problem.New(kind, "https://example.test/problems/common.rate_limited", "trace-1", problem.Parameters{
		"retryAfterSeconds": 30,
		"scopes":            []string{"read", "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.Code != kind.Code() || value.Status != kind.Status() {
		t.Fatalf("problem = %#v, kind = %s/%d", value, kind.Code(), kind.Status())
	}
}

func TestDecodeAllowsExtensionsAndRejectsTrailingJSON(t *testing.T) {
	t.Parallel()
	data := []byte(`{"type":"https://example.test/problems/common.internal","status":500,"code":"common.internal","traceId":"trace-1","extension":true}`)
	if _, err := problem.Decode(data); err != nil {
		t.Fatalf("extension: %v", err)
	}
	if _, err := problem.Decode(append(data, []byte(` {}`)...)); err == nil || !strings.Contains(err.Error(), "multiple JSON") {
		t.Fatalf("trailing JSON error = %v", err)
	}
	if _, err := problem.Decode([]byte(`{"type":`)); err == nil {
		t.Fatal("Decode() accepted malformed JSON")
	}
}

func TestValidateRejectsUnsafeCallerParameters(t *testing.T) {
	t.Parallel()
	kind := problem.MustKind("common.internal", 500)
	tests := []problem.Parameters{
		{"nested": map[string]any{"secret": true}},
		{"null": nil},
		{"nan": math.NaN()},
		{"tooLong": strings.Repeat("x", 1025)},
	}
	for _, params := range tests {
		if _, err := problem.New(kind, "https://example.test/problems/common.internal", "trace-1", params); err == nil {
			t.Fatalf("New() accepted unsafe params %#v", params)
		}
	}
}
