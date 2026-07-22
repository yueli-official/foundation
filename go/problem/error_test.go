package problem

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestMappedErrorIsImmutableAndResolvesPerTrace(t *testing.T) {
	descriptor := MustDescriptor(
		MustKind("catalog.slug_taken", http.StatusConflict),
		"https://errors.example.test/problems/catalog.slug_taken",
	)
	values := []string{"first"}
	params := Parameters{"slug": "hello", "values": values}
	mapped, err := WrapError(descriptor, errors.New("private database detail"), params)
	if err != nil {
		t.Fatal(err)
	}
	params["slug"] = "mutated"
	values[0] = "mutated"

	value, ok, err := FromError(mapped, "trace-one")
	if err != nil || !ok {
		t.Fatalf("FromError() = %#v, %v, %v", value, ok, err)
	}
	if value.Params["slug"] != "hello" || value.Params["values"].([]string)[0] != "first" {
		t.Fatalf("mapped params mutated: %#v", value.Params)
	}
	if value.TraceID != "trace-one" || value.Status != http.StatusConflict {
		t.Fatalf("unexpected Problem: %#v", value)
	}
	if !strings.Contains(mapped.Error(), "private database detail") {
		t.Fatal("wrapped diagnostic is unavailable to server logs")
	}
}

func TestFromErrorRejectsUnknownErrors(t *testing.T) {
	if _, ok, err := FromError(errors.New("unknown"), "trace-two"); err != nil || ok {
		t.Fatalf("FromError unknown = ok %v, err %v", ok, err)
	}
}
