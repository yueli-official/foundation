package audit

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"time"
)

type ExportFormat string

const ExportNDJSONV1 ExportFormat = "ndjson-v1"

type ExportRequest struct {
	Query  Query
	Format ExportFormat
}

type ExportManifest struct {
	EnvelopeVersion  uint16    `json:"envelopeVersion"`
	DefinitionDigest Digest    `json:"definitionDigest"`
	Count            uint64    `json:"count"`
	FirstSequence    Sequence  `json:"firstSequence,omitempty"`
	LastSequence     Sequence  `json:"lastSequence,omitempty"`
	HeadDigest       Digest    `json:"headDigest,omitempty"`
	ContentDigest    Digest    `json:"contentDigest"`
	GeneratedAt      time.Time `json:"generatedAt"`
}

type VerifyRequest struct{}

type VerifyResult struct {
	Valid        bool
	Count        uint64
	FirstInvalid Sequence
	HeadDigest   Digest
}

func (m *Memory) Export(_ context.Context, request ExportRequest, writer io.Writer) (ExportManifest, error) {
	if writer == nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "is required"}
	}
	if request.Format == "" {
		request.Format = ExportNDJSONV1
	}
	if request.Format != ExportNDJSONV1 || request.Query.Before != "" || request.Query.Limit != 0 {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "request", Message: "is invalid"}
	}
	query := request.Query
	query.Limit = 500
	query, _, err := normalizeQuery(query)
	if err != nil {
		return ExportManifest{}, err
	}
	m.mu.RLock()
	events := cloneEvents(m.events)
	head := m.headDigest
	m.mu.RUnlock()

	generatedAt := m.clock.Now().UTC()
	hash := sha256.New()
	buffered := bufio.NewWriter(io.MultiWriter(writer, hash))
	encoder := json.NewEncoder(buffered)
	if err := encoder.Encode(map[string]any{
		"kind": "audit.export.header", "version": 1,
		"definitionDigest": m.catalog.digest, "generatedAt": generatedAt,
	}); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "write failed"}
	}
	manifest := ExportManifest{
		EnvelopeVersion: 1, DefinitionDigest: m.catalog.digest,
		GeneratedAt: generatedAt, HeadDigest: head,
	}
	for _, event := range events {
		if !matchesQuery(event, query) {
			continue
		}
		if manifest.Count == 0 {
			manifest.FirstSequence = event.Sequence
		}
		manifest.Count++
		manifest.LastSequence = event.Sequence
		if err := encoder.Encode(struct {
			Kind  string `json:"kind"`
			Event Event  `json:"event"`
		}{"audit.event", event}); err != nil {
			return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "write failed"}
		}
	}
	if err := encoder.Encode(map[string]any{
		"kind": "audit.export.footer", "count": manifest.Count,
		"firstSequence": manifest.FirstSequence, "lastSequence": manifest.LastSequence,
		"headDigest": manifest.HeadDigest,
	}); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "write failed"}
	}
	if err := buffered.Flush(); err != nil {
		return ExportManifest{}, &Error{Kind: ErrorExportFailed, Field: "writer", Message: "flush failed"}
	}
	manifest.ContentDigest = Digest(hex.EncodeToString(hash.Sum(nil)))
	return manifest, nil
}

func (m *Memory) Verify(_ context.Context, _ VerifyRequest) (VerifyResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ranges := make([]SequenceRange, 0)
	for _, receipt := range m.retentions {
		ranges = append(ranges, receipt.Ranges...)
	}
	return verifyJournal(m.events, ranges, m.head, m.headDigest), nil
}

func verifyJournal(events []Event, ranges []SequenceRange, head Sequence, headDigest Digest) VerifyResult {
	result := VerifyResult{Valid: true, Count: uint64(len(events)), HeadDigest: headDigest}
	slices.SortFunc(ranges, func(left, right SequenceRange) int {
		if left.First < right.First {
			return -1
		}
		if left.First > right.First {
			return 1
		}
		return 0
	})
	var previous Digest
	next := Sequence(1)
	rangeIndex := 0
	bridge := func(until Sequence) bool {
		for next < until {
			if rangeIndex >= len(ranges) {
				return false
			}
			item := ranges[rangeIndex]
			if item.First != next || item.Last < item.First || item.Last >= until ||
				item.PreviousDigest != previous {
				return false
			}
			previous = item.LastDigest
			next = item.Last + 1
			rangeIndex++
		}
		return next == until
	}
	for _, event := range events {
		if event.Sequence < next || !bridge(event.Sequence) {
			result.Valid = false
			result.FirstInvalid = next
			return result
		}
		expected, err := eventDigest(event)
		if err != nil || event.PreviousDigest != previous || event.Digest != expected {
			result.Valid = false
			result.FirstInvalid = event.Sequence
			return result
		}
		previous = event.Digest
		next = event.Sequence + 1
	}
	if !bridge(head+1) || rangeIndex != len(ranges) || previous != headDigest {
		result.Valid = false
		result.FirstInvalid = next
	}
	return result
}
