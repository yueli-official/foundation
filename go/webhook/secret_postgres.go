package webhook

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"
)

type PostgresSecretStore struct {
	db          *sql.DB
	instanceKey string
	aead        cipher.AEAD
	clock       func() time.Time
}

func NewPostgresSecretStore(
	db *sql.DB,
	instanceKey string,
	masterKey []byte,
	clock func() time.Time,
) (*PostgresSecretStore, error) {
	if db == nil {
		return nil, invalid(ErrorInvalidDefinition, "db", "is required")
	}
	instanceKey = strings.TrimSpace(instanceKey)
	if instanceKey == "" || len(instanceKey) > 200 {
		return nil, invalid(ErrorInvalidDefinition, "instance_key", "is invalid")
	}
	if len(masterKey) != 32 {
		return nil, invalid(ErrorInvalidDefinition, "master_key", "must contain exactly 32 bytes")
	}
	block, err := aes.NewCipher(append([]byte(nil), masterKey...))
	if err != nil {
		return nil, &Error{Code: ErrorSecretUnavailable, Field: "master_key", Message: "cannot initialize cipher", Cause: err}
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, &Error{Code: ErrorSecretUnavailable, Field: "master_key", Message: "cannot initialize AEAD", Cause: err}
	}
	if clock == nil {
		clock = time.Now
	}
	return &PostgresSecretStore{db: db, instanceKey: instanceKey, aead: aead, clock: clock}, nil
}

func (store *PostgresSecretStore) Create(ctx context.Context, ref SecretRef, material SecretMaterial) error {
	if err := validateSecretMaterial(ref, material); err != nil {
		return err
	}
	nonce, ciphertext, err := store.encrypt(ref, material)
	if err != nil {
		return err
	}
	_, err = store.db.ExecContext(ctx, `
INSERT INTO webhook_secret_material(
 instance_key,secret_ref,revision,role,nonce,ciphertext,not_before,not_after,created_at
) VALUES($1,$2,$3,'primary',$4,$5,$6,$7,$8)`,
		store.instanceKey, ref, material.Revision, nonce, ciphertext,
		material.NotBefore, material.NotAfter, store.clock().UTC(),
	)
	if err != nil {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot persist revision", Cause: err}
	}
	return nil
}

func (store *PostgresSecretStore) Resolve(ctx context.Context, ref SecretRef, at time.Time) (SecretSet, error) {
	rows, err := store.db.QueryContext(ctx, `
SELECT revision,role,nonce,ciphertext,not_before,not_after
FROM webhook_secret_material
WHERE instance_key=$1 AND secret_ref=$2
  AND not_before <= $3 AND (not_after IS NULL OR not_after > $3)
ORDER BY CASE role WHEN 'primary' THEN 0 ELSE 1 END, created_at DESC`,
		store.instanceKey, ref, at.UTC(),
	)
	if err != nil {
		return SecretSet{}, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot load revisions", Retryable: true, Cause: err}
	}
	defer rows.Close()
	var result SecretSet
	for rows.Next() {
		var material SecretMaterial
		var role string
		var nonce, ciphertext []byte
		var notAfter sql.NullTime
		if err := rows.Scan(&material.Revision, &role, &nonce, &ciphertext, &material.NotBefore, &notAfter); err != nil {
			return SecretSet{}, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot scan revision", Retryable: true, Cause: err}
		}
		if notAfter.Valid {
			material.NotAfter = &notAfter.Time
		}
		material.Value, err = store.decrypt(ref, material.Revision, nonce, ciphertext)
		if err != nil {
			return SecretSet{}, err
		}
		if role == "primary" {
			if len(result.Primary.Value) != 0 {
				return SecretSet{}, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "multiple primary revisions"}
			}
			result.Primary = material
		} else {
			result.Previous = append(result.Previous, material)
		}
	}
	if err := rows.Err(); err != nil {
		return SecretSet{}, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot iterate revisions", Retryable: true, Cause: err}
	}
	if len(result.Primary.Value) == 0 {
		return SecretSet{}, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "active primary revision not found", Retryable: true}
	}
	if len(result.Previous) > 1 {
		return SecretSet{}, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "too many active previous revisions"}
	}
	return result, nil
}

func (store *PostgresSecretStore) Rotate(
	ctx context.Context,
	ref SecretRef,
	material SecretMaterial,
	previousUntil time.Time,
) error {
	if err := validateSecretMaterial(ref, material); err != nil {
		return err
	}
	if !previousUntil.After(material.NotBefore) {
		return invalid(ErrorInvalidEvent, "previous_until", "must be after the new revision start")
	}
	nonce, ciphertext, err := store.encrypt(ref, material)
	if err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot begin rotation", Retryable: true, Cause: err}
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
UPDATE webhook_secret_material
SET not_after=$3
WHERE instance_key=$1 AND secret_ref=$2 AND role='previous'
  AND (not_after IS NULL OR not_after>$3)`,
		store.instanceKey, ref, material.NotBefore.UTC(),
	); err != nil {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot expire older previous revision", Cause: err}
	}
	result, err := tx.ExecContext(ctx, `
UPDATE webhook_secret_material
SET role='previous',not_after=$3
WHERE instance_key=$1 AND secret_ref=$2 AND role='primary'`,
		store.instanceKey, ref, previousUntil.UTC(),
	)
	if err != nil {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot retire primary", Cause: err}
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "primary revision not found", Cause: err}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO webhook_secret_material(
 instance_key,secret_ref,revision,role,nonce,ciphertext,not_before,not_after,created_at
) VALUES($1,$2,$3,'primary',$4,$5,$6,$7,$8)`,
		store.instanceKey, ref, material.Revision, nonce, ciphertext,
		material.NotBefore, material.NotAfter, store.clock().UTC(),
	); err != nil {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot insert primary", Cause: err}
	}
	if err := tx.Commit(); err != nil {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot commit rotation", Retryable: true, Cause: err}
	}
	return nil
}

func (store *PostgresSecretStore) Delete(ctx context.Context, ref SecretRef, revision SecretRevision) error {
	result, err := store.db.ExecContext(ctx, `
DELETE FROM webhook_secret_material
WHERE instance_key=$1 AND secret_ref=$2 AND revision=$3 AND role='primary'
  AND NOT EXISTS (
    SELECT 1 FROM webhook_secret_material sibling
    WHERE sibling.instance_key=$1 AND sibling.secret_ref=$2 AND sibling.revision<>$3
  )`,
		store.instanceKey, ref, revision,
	)
	if err != nil {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot delete orphan revision", Cause: err}
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot inspect orphan deletion", Cause: err}
	}
	if affected > 1 {
		return &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "orphan deletion affected multiple revisions"}
	}
	return nil
}

func validateSecretMaterial(ref SecretRef, material SecretMaterial) error {
	if !stableKey.MatchString(string(ref)) || !stableKey.MatchString(string(material.Revision)) {
		return invalid(ErrorInvalidEvent, "secret", "reference or revision is invalid")
	}
	if len(material.Value) < 24 || len(material.Value) > 64 || material.NotBefore.IsZero() {
		return invalid(ErrorInvalidEvent, "secret", "material is invalid")
	}
	if material.NotAfter != nil && !material.NotAfter.After(material.NotBefore) {
		return invalid(ErrorInvalidEvent, "secret", "validity interval is invalid")
	}
	return nil
}

func (store *PostgresSecretStore) encrypt(ref SecretRef, material SecretMaterial) ([]byte, []byte, error) {
	nonce := make([]byte, store.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot generate nonce", Cause: err}
	}
	aad := []byte(fmt.Sprintf("%s\x00%s\x00%s", store.instanceKey, ref, material.Revision))
	ciphertext := store.aead.Seal(nil, nonce, material.Value, aad)
	return nonce, ciphertext, nil
}

func (store *PostgresSecretStore) decrypt(
	ref SecretRef,
	revision SecretRevision,
	nonce, ciphertext []byte,
) ([]byte, error) {
	aad := []byte(fmt.Sprintf("%s\x00%s\x00%s", store.instanceKey, ref, revision))
	plaintext, err := store.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, &Error{Code: ErrorSecretUnavailable, Field: "secret", Message: "cannot decrypt revision", Cause: err}
	}
	return plaintext, nil
}
