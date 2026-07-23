package urllifecycle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"
)

const archiveFormatVersion uint64 = 1
const maxArchiveBytes int64 = 256 << 20

type archivePayload struct {
	FormatVersion uint64              `json:"formatVersion"`
	PolicyDigest  Digest              `json:"policyDigest"`
	Revision      Revision            `json:"revision"`
	Routes        []archiveRoute      `json:"routes"`
	References    []archiveReference  `json:"references"`
	Overlays      []archiveOverlay    `json:"overlays"`
	Commands      []archiveCommand    `json:"commands"`
	History       []TransitionSummary `json:"history,omitempty"`
}

type archiveEnvelope struct {
	Payload archivePayload `json:"payload"`
	Digest  Digest         `json:"digest"`
}

type archiveRoute struct {
	Key       RouteKey      `json:"key"`
	Active    ActiveRoute   `json:"active"`
	Revision  RouteRevision `json:"revision"`
	ChangedAt time.Time     `json:"changedAt"`
}

type archiveReference struct {
	Kind      referenceKind  `json:"kind"`
	Ref       LocalRef       `json:"ref"`
	Owner     RouteKey       `json:"owner,omitempty"`
	Target    Target         `json:"target,omitempty"`
	Policy    RedirectPolicy `json:"policy,omitempty"`
	ChangedAt time.Time      `json:"changedAt"`
}

type archiveOverlay struct {
	Owner     RouteKey          `json:"owner"`
	Source    LocalRef          `json:"source"`
	Redirect  TemporaryRedirect `json:"redirect"`
	ChangedAt time.Time         `json:"changedAt"`
}

type archiveCommand struct {
	ID      CommandID `json:"id"`
	Digest  Digest    `json:"digest"`
	Receipt Receipt   `json:"receipt"`
}

func exportState(
	catalog *Catalog,
	state registryState,
	query ExportQuery,
	writer io.Writer,
) (ArchiveManifest, error) {
	if writer == nil {
		return ArchiveManifest{}, invalid("writer", "is required")
	}
	payload := archivePayload{
		FormatVersion: archiveFormatVersion, PolicyDigest: catalog.digest, Revision: state.Revision,
		Routes: []archiveRoute{}, References: []archiveReference{},
		Overlays: []archiveOverlay{}, Commands: []archiveCommand{},
	}
	for _, route := range state.Routes {
		payload.Routes = append(payload.Routes, archiveRoute{
			Key: route.Key, Active: activeRouteValue(route), Revision: route.Revision, ChangedAt: route.ChangedAt,
		})
	}
	slices.SortFunc(payload.Routes, func(left, right archiveRoute) int {
		return strings.Compare(routeMapKey(left.Key), routeMapKey(right.Key))
	})
	for _, ref := range state.Refs {
		payload.References = append(payload.References, archiveReference{
			Kind: ref.Kind, Ref: ref.Ref.ref, Owner: ref.Owner, Target: ref.Target,
			Policy: ref.Policy, ChangedAt: ref.ChangedAt,
		})
	}
	slices.SortFunc(payload.References, func(left, right archiveReference) int {
		leftRef, _ := catalog.normalizeLocalRef(left.Ref)
		rightRef, _ := catalog.normalizeLocalRef(right.Ref)
		return strings.Compare(leftRef.key, rightRef.key)
	})
	for _, overlay := range state.Overlays {
		payload.Overlays = append(payload.Overlays, archiveOverlay{
			Owner: overlay.Owner, Source: overlay.Source.ref,
			Redirect: overlay.Redirect, ChangedAt: overlay.ChangedAt,
		})
	}
	slices.SortFunc(payload.Overlays, func(left, right archiveOverlay) int {
		leftRef, _ := catalog.normalizeLocalRef(left.Source)
		rightRef, _ := catalog.normalizeLocalRef(right.Source)
		return strings.Compare(leftRef.key, rightRef.key)
	})
	for id, command := range state.Commands {
		payload.Commands = append(payload.Commands, archiveCommand{
			ID: id, Digest: command.Digest, Receipt: command.Receipt,
		})
	}
	slices.SortFunc(payload.Commands, func(left, right archiveCommand) int {
		return strings.Compare(string(left.ID), string(right.ID))
	})
	if query.IncludeAudit {
		payload.History = append([]TransitionSummary(nil), state.History...)
	}
	digest, _, err := archiveDigest(payload)
	if err != nil {
		return ArchiveManifest{}, err
	}
	envelope := archiveEnvelope{Payload: payload, Digest: digest}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(envelope); err != nil {
		return ArchiveManifest{}, &Error{Kind: ErrorUnavailable, Field: "archive", Message: "cannot write archive", Cause: err}
	}
	return ArchiveManifest{
		FormatVersion: archiveFormatVersion, PolicyDigest: catalog.digest,
		Revision: state.Revision,
		Records:  uint64(len(payload.Routes) + len(payload.References) + len(payload.Overlays) + len(payload.Commands)),
		Digest:   digest,
	}, nil
}

func verifyArchive(catalog *Catalog, reader io.Reader) (ArchiveReport, error) {
	envelope, encoded, err := decodeArchive(reader)
	if err != nil {
		return ArchiveReport{}, err
	}
	digest, _, err := archiveDigest(envelope.Payload)
	if err != nil {
		return ArchiveReport{}, err
	}
	report := ArchiveReport{
		Valid: true,
		Manifest: ArchiveManifest{
			FormatVersion: envelope.Payload.FormatVersion,
			PolicyDigest:  envelope.Payload.PolicyDigest,
			Revision:      envelope.Payload.Revision,
			Records: uint64(len(envelope.Payload.Routes) + len(envelope.Payload.References) +
				len(envelope.Payload.Overlays) + len(envelope.Payload.Commands)),
			Digest: envelope.Digest,
		},
	}
	if envelope.Payload.FormatVersion != archiveFormatVersion {
		report.Valid = false
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: "format_version", Message: "archive format version is incompatible"})
	}
	if envelope.Payload.PolicyDigest != catalog.digest {
		report.Valid = false
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: "policy_digest", Message: "archive policy digest does not match"})
	}
	if envelope.Digest != digest {
		report.Valid = false
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: "digest", Message: "archive digest does not match content"})
	}
	if _, err := stateFromArchive(catalog, envelope.Payload); err != nil {
		report.Valid = false
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: "state", Message: err.Error()})
	}
	_ = encoded
	return report, nil
}

func restoreArchive(
	catalog *Catalog,
	current registryState,
	command RestoreCommand,
	reader io.Reader,
	now time.Time,
) (registryState, RestoreReport, error) {
	envelope, _, err := decodeArchive(reader)
	if err != nil {
		return current, RestoreReport{}, err
	}
	report, err := verifyArchive(catalog, bytes.NewReader(mustEncodeArchive(envelope)))
	if err != nil {
		return current, RestoreReport{}, err
	}
	if !report.Valid {
		return current, RestoreReport{}, &Error{
			Kind: ErrorIncompatibleArchive, Field: "archive",
			Message: "archive verification failed", Diagnostics: report.Diagnostics,
		}
	}
	if command.RequireEmpty && (len(current.Routes) != 0 || len(current.Refs) != 0 || current.Revision != 0) {
		return current, RestoreReport{}, conflict("archive", "registry is not empty")
	}
	next, err := stateFromArchive(catalog, envelope.Payload)
	if err != nil {
		return current, RestoreReport{}, err
	}
	plan := Plan{
		Valid: true, BaseRevision: current.Revision, IntentDigest: envelope.Digest,
		Effects: archiveEffects(next),
	}
	result := RestoreReport{Plan: plan, Manifest: report.Manifest}
	if command.DryRun {
		return current, result, nil
	}
	if strings.TrimSpace(string(command.CommandID)) == "" || strings.TrimSpace(command.Reason) == "" {
		return current, RestoreReport{}, invalid("restore", "command id and reason are required")
	}
	receipt := Receipt{
		CommandID: command.CommandID, IntentDigest: envelope.Digest,
		Revision: next.Revision, Effects: plan.Effects,
	}
	next.Commands[command.CommandID] = storedCommand{Digest: envelope.Digest, Receipt: receipt}
	next.History = append(next.History, TransitionSummary{
		CommandID: command.CommandID, Revision: next.Revision, Actor: command.Actor,
		Reason: command.Reason, AppliedAt: now,
	})
	result.Receipt = &receipt
	return next, result, nil
}

func rebuildState(catalog *Catalog, state *registryState, command RebuildCommand) (RebuildReport, error) {
	if command.ExpectedHead != 0 && command.ExpectedHead != state.Revision {
		return RebuildReport{}, &Error{Kind: ErrorStaleRevision, Field: "expected_head", Message: "revision no longer matches"}
	}
	if err := validateFinalState(catalog, *state); err != nil {
		return RebuildReport{}, err
	}
	return RebuildReport{
		Revision: state.Revision,
		Records:  uint64(len(state.Routes) + len(state.Refs) + len(state.Overlays)),
		Changed:  false,
	}, nil
}

func decodeArchive(reader io.Reader) (archiveEnvelope, []byte, error) {
	if reader == nil {
		return archiveEnvelope{}, nil, invalid("reader", "is required")
	}
	encoded, err := io.ReadAll(io.LimitReader(reader, maxArchiveBytes+1))
	if err != nil {
		return archiveEnvelope{}, nil, &Error{Kind: ErrorUnavailable, Field: "archive", Message: "cannot read archive", Cause: err}
	}
	if int64(len(encoded)) > maxArchiveBytes {
		return archiveEnvelope{}, nil, &Error{Kind: ErrorLimitExceeded, Field: "archive", Message: "archive is too large"}
	}
	var envelope archiveEnvelope
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return archiveEnvelope{}, nil, &Error{Kind: ErrorIncompatibleArchive, Field: "archive", Message: "cannot decode archive", Cause: err}
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return archiveEnvelope{}, nil, &Error{Kind: ErrorIncompatibleArchive, Field: "archive", Message: "contains trailing data"}
	}
	return envelope, encoded, nil
}

func archiveDigest(payload archivePayload) (Digest, []byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", nil, &Error{Kind: ErrorUnavailable, Field: "archive", Message: "cannot encode archive", Cause: err}
	}
	sum := sha256.Sum256(encoded)
	return Digest(hex.EncodeToString(sum[:])), encoded, nil
}

func mustEncodeArchive(envelope archiveEnvelope) []byte {
	encoded, _ := json.Marshal(envelope)
	return encoded
}

func stateFromArchive(catalog *Catalog, payload archivePayload) (registryState, error) {
	if payload.FormatVersion != archiveFormatVersion || payload.PolicyDigest != catalog.digest {
		return registryState{}, &Error{Kind: ErrorIncompatibleArchive, Field: "archive", Message: "format or policy is incompatible"}
	}
	state := emptyState()
	state.Revision = payload.Revision
	state.History = append([]TransitionSummary(nil), payload.History...)
	for _, item := range payload.Routes {
		if err := catalog.validateRouteKey(item.Key); err != nil {
			return registryState{}, invalid("archive.route", "%v", err)
		}
		canonical, err := catalog.normalizeLocalRef(item.Active.Canonical)
		if err != nil {
			return registryState{}, err
		}
		route := storedRoute{
			Key: item.Key, Canonical: canonical, Aliases: map[string]storedAlias{},
			Revision: item.Revision, ChangedAt: item.ChangedAt,
		}
		for _, alias := range item.Active.Aliases {
			ref, err := catalog.normalizeLocalRef(alias.Ref)
			if err != nil {
				return registryState{}, err
			}
			route.Aliases[ref.key] = storedAlias{Ref: ref, Policy: alias.Policy}
		}
		key := routeMapKey(item.Key)
		if _, exists := state.Routes[key]; exists {
			return registryState{}, conflict("archive.route", "contains duplicate route")
		}
		state.Routes[key] = route
	}
	for _, item := range payload.References {
		ref, err := catalog.normalizeLocalRef(item.Ref)
		if err != nil {
			return registryState{}, err
		}
		if _, exists := state.Refs[ref.key]; exists {
			return registryState{}, conflict("archive.reference", "contains duplicate reference")
		}
		state.Refs[ref.key] = storedReference{
			Ref: ref, Kind: item.Kind, Owner: item.Owner, Target: item.Target,
			Policy: item.Policy, ChangedAt: item.ChangedAt,
		}
	}
	for _, item := range payload.Overlays {
		ref, err := catalog.normalizeLocalRef(item.Source)
		if err != nil {
			return registryState{}, err
		}
		if _, exists := state.Overlays[ref.key]; exists {
			return registryState{}, conflict("archive.overlay", "contains duplicate overlay")
		}
		state.Overlays[ref.key] = storedOverlay{
			Owner: item.Owner, Source: ref, Redirect: item.Redirect, ChangedAt: item.ChangedAt,
		}
	}
	for _, item := range payload.Commands {
		if _, exists := state.Commands[item.ID]; exists {
			return registryState{}, conflict("archive.command", "contains duplicate command")
		}
		state.Commands[item.ID] = storedCommand{Digest: item.Digest, Receipt: item.Receipt}
	}
	if err := validateFinalState(catalog, state); err != nil {
		return registryState{}, err
	}
	return state, nil
}

func archiveEffects(state registryState) []Effect {
	effects := make([]Effect, 0, len(state.Refs)+len(state.Overlays))
	for _, ref := range state.Refs {
		effect := Effect{Ref: ref.Ref.ref}
		switch ref.Kind {
		case referenceCanonical:
			effect.Kind = EffectClaim
			route := ref.Owner
			effect.Route = &route
		case referenceAlias:
			effect.Kind = EffectAlias
			route := ref.Owner
			effect.Route = &route
		case referenceRedirect:
			effect.Kind = EffectRedirect
			target := ref.Target
			effect.Target = &target
		case referenceGone:
			effect.Kind = EffectGone
		}
		effects = append(effects, effect)
	}
	for _, overlay := range state.Overlays {
		target := overlay.Redirect.Target
		effects = append(effects, Effect{Kind: EffectOverlaySet, Ref: overlay.Source.ref, Target: &target})
	}
	slices.SortFunc(effects, func(left, right Effect) int {
		if compared := strings.Compare(left.Ref.Path, right.Ref.Path); compared != 0 {
			return compared
		}
		return strings.Compare(encodeQuery(left.Ref.Query), encodeQuery(right.Ref.Query))
	})
	return effects
}
