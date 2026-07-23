package discovery

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
)

// MemorySource is a deterministic reference Adapter. Records must already be
// sorted by SortKey.
type MemorySource struct {
	Records []Record
}

func (source MemorySource) Next(_ context.Context, after Cursor, limit int) (Batch, error) {
	start := 0
	if after != "" {
		parsed, err := strconv.Atoi(string(after))
		if err != nil || parsed < 0 || parsed > len(source.Records) {
			return Batch{}, fmt.Errorf("invalid memory cursor")
		}
		start = parsed
	}
	if limit < 1 {
		return Batch{}, fmt.Errorf("limit must be positive")
	}
	end := min(start+limit, len(source.Records))
	records := append([]Record(nil), source.Records[start:end]...)
	return Batch{
		Records: records, NextCursor: Cursor(strconv.Itoa(end)),
		Done: end == len(source.Records),
	}, nil
}

type MemoryPublication struct {
	Manifest   PublicationManifest
	Artifacts  map[string][]byte
	MediaTypes map[string]string
}

// MemoryTarget atomically exposes only the last committed publication.
type MemoryTarget struct {
	mu          sync.RWMutex
	publication MemoryPublication
}

func (target *MemoryTarget) Begin(context.Context) (PublicationWriter, error) {
	if target == nil {
		return nil, fmt.Errorf("memory target is nil")
	}
	return &memoryTransaction{
		target: target, artifacts: map[string][]byte{}, mediaTypes: map[string]string{},
		open: map[string]*memoryArtifactWriter{},
	}, nil
}

func (target *MemoryTarget) Snapshot() MemoryPublication {
	target.mu.RLock()
	defer target.mu.RUnlock()
	result := MemoryPublication{
		Manifest:   target.publication.Manifest,
		Artifacts:  make(map[string][]byte, len(target.publication.Artifacts)),
		MediaTypes: make(map[string]string, len(target.publication.MediaTypes)),
	}
	for name, value := range target.publication.Artifacts {
		result.Artifacts[name] = append([]byte(nil), value...)
	}
	for name, value := range target.publication.MediaTypes {
		result.MediaTypes[name] = value
	}
	return result
}

type memoryTransaction struct {
	mu         sync.Mutex
	target     *MemoryTarget
	artifacts  map[string][]byte
	mediaTypes map[string]string
	open       map[string]*memoryArtifactWriter
	finished   bool
}

func (transaction *memoryTransaction) Create(_ context.Context, name, mediaType string) (io.WriteCloser, error) {
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	if transaction.finished {
		return nil, fmt.Errorf("transaction is finished")
	}
	if _, exists := transaction.open[name]; exists {
		return nil, fmt.Errorf("artifact %q already exists", name)
	}
	writer := &memoryArtifactWriter{transaction: transaction, name: name, mediaType: mediaType}
	transaction.open[name] = writer
	return writer, nil
}

func (transaction *memoryTransaction) Commit(_ context.Context, manifest PublicationManifest) error {
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	if transaction.finished {
		return fmt.Errorf("transaction is finished")
	}
	if len(transaction.open) != 0 {
		return fmt.Errorf("artifacts remain open")
	}
	transaction.finished = true
	transaction.target.mu.Lock()
	defer transaction.target.mu.Unlock()
	transaction.target.publication = MemoryPublication{
		Manifest: manifest, Artifacts: transaction.artifacts, MediaTypes: transaction.mediaTypes,
	}
	return nil
}

func (transaction *memoryTransaction) Abort(context.Context, error) error {
	transaction.mu.Lock()
	defer transaction.mu.Unlock()
	transaction.finished = true
	transaction.open = map[string]*memoryArtifactWriter{}
	transaction.artifacts = map[string][]byte{}
	transaction.mediaTypes = map[string]string{}
	return nil
}

type memoryArtifactWriter struct {
	transaction *memoryTransaction
	name        string
	mediaType   string
	buffer      bytes.Buffer
	closed      bool
}

func (writer *memoryArtifactWriter) Write(value []byte) (int, error) {
	if writer.closed {
		return 0, fmt.Errorf("artifact is closed")
	}
	return writer.buffer.Write(value)
}

func (writer *memoryArtifactWriter) Close() error {
	writer.transaction.mu.Lock()
	defer writer.transaction.mu.Unlock()
	if writer.closed {
		return fmt.Errorf("artifact is already closed")
	}
	if writer.transaction.finished {
		return fmt.Errorf("transaction is finished")
	}
	writer.closed = true
	writer.transaction.artifacts[writer.name] = append([]byte(nil), writer.buffer.Bytes()...)
	writer.transaction.mediaTypes[writer.name] = writer.mediaType
	delete(writer.transaction.open, writer.name)
	return nil
}
