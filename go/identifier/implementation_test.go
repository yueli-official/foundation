package identifier

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
)

func TestRandomTextRejectsModuloBiasSamples(t *testing.T) {
	// Base58 accepts bytes below 232. The leading 255 must be discarded rather
	// than folded with modulo, after which zero maps to the first alphabet rune.
	value, err := randomText(&byteReader{values: append([]byte{255, 0}, make([]byte, 14)...)}, base58Alphabet, 1)
	if err != nil {
		t.Fatalf("randomText: %v", err)
	}
	if value != "1" {
		t.Fatalf("randomText = %q, want first Base58 rune", value)
	}
}

func TestRandomTextReportsEntropyFailure(t *testing.T) {
	want := errors.New("entropy offline")
	_, err := randomText(errorReader{err: want}, base32Alphabet, 8)
	if !errors.Is(err, ErrEntropyUnavailable) {
		t.Fatalf("randomText error = %v, want ErrEntropyUnavailable", err)
	}
	// The public error remains stable and does not expose an implementation
	// sentinel for caller branching.
	if errors.Is(err, want) {
		t.Fatalf("randomText leaked implementation error identity: %v", err)
	}
}

func TestRepositoryContractMatchesPublishedProfiles(t *testing.T) {
	raw, err := os.ReadFile("../../contracts/identifier/profiles.v1.json")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("repository-level cross-language contract is outside the published Go module")
	}
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var contract struct {
		SchemaVersion int `json:"schemaVersion"`
		Profiles      []struct {
			ID       string `json:"id"`
			Alphabet string `json:"alphabet"`
			Length   int    `json:"length"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	if contract.SchemaVersion != 1 || len(contract.Profiles) != 3 {
		t.Fatalf("contract version=%d profiles=%d", contract.SchemaVersion, len(contract.Profiles))
	}
	want := []keyProfileDefinition{
		{name: CompactURLV1.Name(), alphabet: base58Alphabet, length: 8},
		{name: HumanCodeV1.Name(), alphabet: base32Alphabet, length: 10},
		{name: OpaquePublicV1.Name(), alphabet: base32Alphabet, length: 16},
	}
	for index, profile := range contract.Profiles {
		if profile.ID != want[index].name || profile.Alphabet != want[index].alphabet || profile.Length != want[index].length {
			t.Fatalf("contract profile[%d] = %#v, want %#v", index, profile, want[index])
		}
	}
}

func TestRepositoryContractMatchesDeterministicVectors(t *testing.T) {
	raw, err := os.ReadFile("../../contracts/identifier/derive.v1.json")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("repository-level cross-language contract is outside the published Go module")
	}
	if err != nil {
		t.Fatalf("read deterministic contract: %v", err)
	}
	var contract struct {
		SchemaVersion int    `json:"schemaVersion"`
		Algorithm     string `json:"algorithm"`
		Vectors       []struct {
			Namespace     string `json:"namespace"`
			CanonicalName string `json:"canonicalNameUTF8"`
			Identifier    string `json:"identifier"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("decode deterministic contract: %v", err)
	}
	if contract.SchemaVersion != 1 || contract.Algorithm != "uuid-v5" || len(contract.Vectors) == 0 {
		t.Fatalf("deterministic contract version=%d algorithm=%q vectors=%d", contract.SchemaVersion, contract.Algorithm, len(contract.Vectors))
	}
	for index, vector := range contract.Vectors {
		namespace, err := Parse(vector.Namespace)
		if err != nil {
			t.Fatalf("vector[%d] namespace: %v", index, err)
		}
		if got := Derive(namespace, []byte(vector.CanonicalName)).String(); got != vector.Identifier {
			t.Fatalf("vector[%d] identifier=%q, want %q", index, got, vector.Identifier)
		}
	}
}

type byteReader struct {
	values []byte
}

func (r *byteReader) Read(target []byte) (int, error) {
	if len(r.values) == 0 {
		return 0, io.EOF
	}
	count := copy(target, r.values)
	r.values = r.values[count:]
	return count, nil
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
