package siteprofile

import (
	"context"
	"time"
)

type StoredState struct {
	Document      []byte
	Digest        Digest
	Revision      Revision
	SchemaVersion uint64
	UpdatedAt     time.Time
}

type Store interface {
	Load(context.Context) (StoredState, bool, error)
	CompareAndSwap(context.Context, Revision, StoredState) (bool, Revision, error)
}
