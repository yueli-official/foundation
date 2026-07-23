package siteprofile

import (
	"context"
	"slices"
	"sync"
)

type MemoryStore struct {
	mu    sync.RWMutex
	state StoredState
	found bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func NewMemory(definition CompiledDefinition, clock Clock) *Service {
	return MustNew(NewMemoryStore(), definition, clock)
}

func (s *MemoryStore) Load(context.Context) (StoredState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.found {
		return StoredState{}, false, nil
	}
	return cloneStoredState(s.state), true, nil
}

func (s *MemoryStore) CompareAndSwap(_ context.Context, expected Revision, next StoredState) (bool, Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	actual := Revision(0)
	if s.found {
		actual = s.state.Revision
	}
	if actual != expected {
		return false, actual, nil
	}
	s.state = cloneStoredState(next)
	s.found = true
	return true, next.Revision, nil
}

func cloneStoredState(in StoredState) StoredState {
	in.Document = slices.Clone(in.Document)
	return in
}
