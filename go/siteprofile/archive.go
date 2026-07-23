package siteprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const archiveFormatVersion uint64 = 1
const maxArchiveBytes int64 = 2 << 20

type archiveEnvelope struct {
	FormatVersion  uint64    `json:"formatVersion"`
	SchemaVersion  uint64    `json:"schemaVersion"`
	Revision       Revision  `json:"revision"`
	DocumentDigest Digest    `json:"documentDigest"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Profile        Profile   `json:"profile"`
	ArchiveDigest  Digest    `json:"archiveDigest"`
}

func (s *Service) Export(ctx context.Context, writer io.Writer) (ArchiveManifest, error) {
	if writer == nil {
		return ArchiveManifest{}, errors.New("siteprofile: archive writer is required")
	}
	snapshot, err := s.Get(ctx)
	if err != nil {
		return ArchiveManifest{}, err
	}
	envelope := archiveEnvelope{
		FormatVersion: archiveFormatVersion, SchemaVersion: snapshot.SchemaVersion,
		Revision: snapshot.Revision, DocumentDigest: snapshot.DocumentDigest,
		UpdatedAt: snapshot.UpdatedAt, Profile: snapshot.Profile,
	}
	digest, err := digestArchive(envelope)
	if err != nil {
		return ArchiveManifest{}, err
	}
	envelope.ArchiveDigest = digest
	if err := json.NewEncoder(writer).Encode(envelope); err != nil {
		return ArchiveManifest{}, fmt.Errorf("siteprofile: encode archive: %w", err)
	}
	return manifestFromEnvelope(envelope), nil
}

func (s *Service) VerifyArchive(reader io.Reader) (ArchiveReport, error) {
	envelope, report, err := s.readArchive(reader)
	if err != nil {
		return ArchiveReport{}, err
	}
	report.Manifest = manifestFromEnvelope(envelope)
	return report, nil
}

func (s *Service) Restore(ctx context.Context, command RestoreCommand, reader io.Reader) (RestoreResult, error) {
	envelope, report, err := s.readArchive(reader)
	if err != nil {
		return RestoreResult{}, err
	}
	if !report.Valid {
		return RestoreResult{Manifest: manifestFromEnvelope(envelope)}, &ValidationError{Diagnostics: report.Diagnostics}
	}
	out := RestoreResult{Manifest: manifestFromEnvelope(envelope)}
	if command.DryRun {
		return out, nil
	}
	result, err := s.Replace(ctx, ReplaceCommand{
		ExpectedRevision: command.ExpectedRevision,
		Profile:          envelope.Profile,
	})
	if err != nil {
		return RestoreResult{}, err
	}
	out.Result = &result
	return out, nil
}

func (s *Service) readArchive(reader io.Reader) (archiveEnvelope, ArchiveReport, error) {
	if reader == nil {
		return archiveEnvelope{}, ArchiveReport{}, errors.New("siteprofile: archive reader is required")
	}
	limited := &io.LimitedReader{R: reader, N: maxArchiveBytes + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	var envelope archiveEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return archiveEnvelope{}, ArchiveReport{}, fmt.Errorf("siteprofile: decode archive: %w", err)
	}
	if limited.N <= 0 {
		return archiveEnvelope{}, ArchiveReport{}, errors.New("siteprofile: archive exceeds size limit")
	}
	if err := ensureArchiveEOF(decoder); err != nil {
		return archiveEnvelope{}, ArchiveReport{}, err
	}
	var diagnostics []Diagnostic
	if envelope.FormatVersion != archiveFormatVersion {
		diagnostics = append(diagnostics, Diagnostic{Code: "archive_format", Path: "formatVersion", Message: "archive format is unsupported"})
	}
	if envelope.SchemaVersion != s.definition.value.SchemaVersion {
		diagnostics = append(diagnostics, Diagnostic{Code: "schema_version", Path: "schemaVersion", Message: "archive schema is incompatible"})
	}
	profile := normalizeProfile(envelope.Profile)
	diagnostics = append(diagnostics, validateProfile(profile, s.definition.value)...)
	_, documentDigest, err := encodeProfile(profile)
	if err != nil {
		return archiveEnvelope{}, ArchiveReport{}, err
	}
	if !constantDigestEqual(documentDigest, envelope.DocumentDigest) {
		diagnostics = append(diagnostics, Diagnostic{Code: "document_digest", Path: "documentDigest", Message: "archive document digest does not match"})
	}
	expectedArchiveDigest, err := digestArchive(envelope)
	if err != nil {
		return archiveEnvelope{}, ArchiveReport{}, err
	}
	if !constantDigestEqual(expectedArchiveDigest, envelope.ArchiveDigest) {
		diagnostics = append(diagnostics, Diagnostic{Code: "archive_digest", Path: "archiveDigest", Message: "archive digest does not match"})
	}
	envelope.Profile = profile
	return envelope, ArchiveReport{Valid: len(diagnostics) == 0, Diagnostics: diagnostics}, nil
}

func digestArchive(envelope archiveEnvelope) (Digest, error) {
	envelope.ArchiveDigest = ""
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("siteprofile: encode archive digest input: %w", err)
	}
	sum := sha256.Sum256(raw)
	return Digest(hex.EncodeToString(sum[:])), nil
}

func manifestFromEnvelope(envelope archiveEnvelope) ArchiveManifest {
	return ArchiveManifest{
		FormatVersion: envelope.FormatVersion, SchemaVersion: envelope.SchemaVersion,
		Revision: envelope.Revision, DocumentDigest: envelope.DocumentDigest,
		ArchiveDigest: envelope.ArchiveDigest, UpdatedAt: envelope.UpdatedAt,
	}
}

func ensureArchiveEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("siteprofile: archive contains trailing JSON")
		}
		return fmt.Errorf("siteprofile: decode archive trailer: %w", err)
	}
	return nil
}
