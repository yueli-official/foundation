package webhook

import (
	"bytes"
	"database/sql"
	"testing"
	"time"
)

func TestPostgresSecretStoreAEADBindsInstanceReferenceAndRevision(t *testing.T) {
	store, err := NewPostgresSecretStore(&sql.DB{}, "test-instance", bytes.Repeat([]byte{0x42}, 32), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	material := SecretMaterial{
		Revision: "r1", Value: []byte("secret-value-at-least-24-bytes"),
		NotBefore: time.Now().UTC(),
	}
	nonce, ciphertext, err := store.encrypt("endpoint.primary", material)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := store.decrypt("endpoint.primary", "r1", nonce, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plaintext, material.Value) {
		t.Fatalf("plaintext=%q", plaintext)
	}
	if _, err := store.decrypt("endpoint.other", "r1", nonce, ciphertext); !IsCode(err, ErrorSecretUnavailable) {
		t.Fatalf("changed AAD err=%v", err)
	}
	ciphertext[0] ^= 0xff
	if _, err := store.decrypt("endpoint.primary", "r1", nonce, ciphertext); !IsCode(err, ErrorSecretUnavailable) {
		t.Fatalf("tampered ciphertext err=%v", err)
	}
}

func TestPostgresSecretStoreRequires256BitMasterKey(t *testing.T) {
	if _, err := NewPostgresSecretStore(&sql.DB{}, "test", []byte("short"), time.Now); !IsCode(err, ErrorInvalidDefinition) {
		t.Fatalf("short master key err=%v", err)
	}
}
