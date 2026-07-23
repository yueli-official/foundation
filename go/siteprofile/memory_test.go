package siteprofile

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCorruptMemoryStateFailsClosed(t *testing.T) {
	store := NewMemoryStore()
	store.state = StoredState{
		Document: []byte(`{"identity":{"name":"tampered"}}`),
		Digest:   "wrong", Revision: 1, SchemaVersion: 1, UpdatedAt: time.Now(),
	}
	store.found = true
	service := MustNew(store, MustCompileDefinition(DefaultDefinition()), SystemClock{})
	if _, err := service.Get(context.Background()); !errors.Is(err, ErrCorruptState) {
		t.Fatalf("Get error = %v, want ErrCorruptState", err)
	}
}
