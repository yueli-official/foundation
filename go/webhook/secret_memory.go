package webhook

import (
	"context"
	"sync"
	"time"
)

type MemorySecretStore struct {
	mu      sync.RWMutex
	secrets map[SecretRef]SecretSet
}

func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{secrets: map[SecretRef]SecretSet{}}
}

func (store *MemorySecretStore) Create(ctx context.Context, ref SecretRef, material SecretMaterial) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.secrets[ref]; exists {
		return &Error{Code: ErrorStateConflict, Field: "secret", Message: "already exists"}
	}
	material.Value = append([]byte(nil), material.Value...)
	store.secrets[ref] = SecretSet{Primary: material}
	return nil
}

func (store *MemorySecretStore) Resolve(ctx context.Context, ref SecretRef, at time.Time) (SecretSet, error) {
	if err := ctx.Err(); err != nil {
		return SecretSet{}, err
	}
	store.mu.RLock()
	value, exists := store.secrets[ref]
	store.mu.RUnlock()
	if !exists {
		return SecretSet{}, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "not found", Retryable: true}
	}
	result := SecretSet{Primary: cloneSecret(value.Primary)}
	if !activeSecret(result.Primary, at) {
		return SecretSet{}, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "primary revision is not active", Retryable: true}
	}
	for _, item := range value.Previous {
		if activeSecret(item, at) {
			result.Previous = append(result.Previous, cloneSecret(item))
		}
	}
	return result, nil
}

func (store *MemorySecretStore) Rotate(ctx context.Context, ref SecretRef, material SecretMaterial, previousUntil time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	value, exists := store.secrets[ref]
	if !exists {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "not found"}
	}
	old := cloneSecret(value.Primary)
	old.NotAfter = &previousUntil
	material.Value = append([]byte(nil), material.Value...)
	store.secrets[ref] = SecretSet{Primary: material, Previous: []SecretMaterial{old}}
	return nil
}

func (store *MemorySecretStore) Delete(ctx context.Context, ref SecretRef, revision SecretRevision) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	value, exists := store.secrets[ref]
	if !exists {
		return nil
	}
	if value.Primary.Revision != revision || len(value.Previous) != 0 {
		return &Error{Code: ErrorStateConflict, Field: "secret", Message: "revision is not the sole primary"}
	}
	delete(store.secrets, ref)
	return nil
}

func cloneSecret(value SecretMaterial) SecretMaterial {
	value.Value = append([]byte(nil), value.Value...)
	return value
}

func activeSecret(value SecretMaterial, at time.Time) bool {
	return !at.Before(value.NotBefore) && (value.NotAfter == nil || at.Before(*value.NotAfter))
}
