package siteprofile

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Service struct {
	store      Store
	definition CompiledDefinition
	clock      Clock
}

func New(store Store, definition CompiledDefinition, clock Clock) (*Service, error) {
	if store == nil {
		return nil, errors.New("siteprofile: store is required")
	}
	if definition.value.SchemaVersion == 0 {
		return nil, errors.New("siteprofile: compiled definition is required")
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, definition: definition, clock: clock}, nil
}

func MustNew(store Store, definition CompiledDefinition, clock Clock) *Service {
	service, err := New(store, definition, clock)
	if err != nil {
		panic(err)
	}
	return service
}

func (s *Service) Schema() FormSchema {
	raw, _ := json.Marshal(s.definition.schema)
	var out FormSchema
	_ = json.Unmarshal(raw, &out)
	return out
}

func (s *Service) Get(ctx context.Context) (Snapshot, error) {
	state, found, err := s.store.Load(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if !found {
		return Snapshot{}, ErrNotInitialized
	}
	return s.snapshot(state)
}

func (s *Service) Replace(ctx context.Context, command ReplaceCommand) (ReplaceResult, error) {
	profile := normalizeProfile(command.Profile)
	if diagnostics := validateProfile(profile, s.definition.value); len(diagnostics) != 0 {
		return ReplaceResult{}, &ValidationError{Diagnostics: diagnostics}
	}
	document, digest, err := encodeProfile(profile)
	if err != nil {
		return ReplaceResult{}, err
	}
	current, found, err := s.store.Load(ctx)
	if err != nil {
		return ReplaceResult{}, err
	}
	actual := Revision(0)
	if found {
		actual = current.Revision
	}
	if command.ExpectedRevision != actual {
		return ReplaceResult{}, &RevisionConflictError{Expected: command.ExpectedRevision, Actual: actual}
	}
	if found && constantDigestEqual(current.Digest, digest) {
		snapshot, err := s.snapshot(current)
		return ReplaceResult{Snapshot: snapshot, Changed: false}, err
	}
	next := StoredState{
		Document: document, Digest: digest, Revision: actual + 1,
		SchemaVersion: s.definition.value.SchemaVersion, UpdatedAt: s.clock.Now().UTC(),
	}
	swapped, observed, err := s.store.CompareAndSwap(ctx, actual, next)
	if err != nil {
		return ReplaceResult{}, err
	}
	if !swapped {
		return ReplaceResult{}, &RevisionConflictError{Expected: actual, Actual: observed}
	}
	snapshot, err := s.snapshot(next)
	if err != nil {
		return ReplaceResult{}, err
	}
	return ReplaceResult{Snapshot: snapshot, Changed: true}, nil
}

func (s *Service) PublicAt(ctx context.Context, now time.Time) (PublicProjection, error) {
	snapshot, err := s.Get(ctx)
	if err != nil {
		return PublicProjection{}, err
	}
	profile := cloneProfile(snapshot.Profile)
	announcement := &profile.Announcement
	var next *time.Time
	visible := announcement.Enabled
	if visible && announcement.StartsAt != nil && now.Before(*announcement.StartsAt) {
		visible = false
		value := announcement.StartsAt.UTC()
		next = &value
	}
	if visible && announcement.EndsAt != nil {
		if !now.Before(*announcement.EndsAt) {
			visible = false
		} else {
			value := announcement.EndsAt.UTC()
			next = &value
		}
	}
	announcement.Enabled = visible
	snapshot.Profile = profile
	return PublicProjection{Snapshot: snapshot, NextChangeAt: next}, nil
}

func (s *Service) snapshot(state StoredState) (Snapshot, error) {
	if state.Revision == 0 || state.SchemaVersion != s.definition.value.SchemaVersion || state.UpdatedAt.IsZero() {
		return Snapshot{}, fmt.Errorf("%w: invalid metadata", ErrCorruptState)
	}
	profile, err := decodeProfile(state.Document)
	if err != nil {
		return Snapshot{}, err
	}
	normalized := normalizeProfile(profile)
	if diagnostics := validateProfile(normalized, s.definition.value); len(diagnostics) != 0 {
		return Snapshot{}, fmt.Errorf("%w: persisted profile is invalid: %s", ErrCorruptState, diagnostics[0].Message)
	}
	_, digest, err := encodeProfile(normalized)
	if err != nil {
		return Snapshot{}, err
	}
	if !constantDigestEqual(digest, state.Digest) {
		return Snapshot{}, fmt.Errorf("%w: document digest mismatch", ErrCorruptState)
	}
	return Snapshot{
		Profile: normalized, Revision: state.Revision, ETag: makeETag(state.SchemaVersion, state.Revision, digest),
		DocumentDigest: digest, SchemaVersion: state.SchemaVersion, UpdatedAt: state.UpdatedAt.UTC(),
	}, nil
}

func makeETag(schemaVersion uint64, revision Revision, digest Digest) string {
	value := string(digest)
	if len(value) > 16 {
		value = value[:16]
	}
	return fmt.Sprintf(`"site-profile-v%d-r%d-%s"`, schemaVersion, revision, value)
}

func constantDigestEqual(left, right Digest) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func cloneProfile(in Profile) Profile {
	raw, _ := json.Marshal(in)
	var out Profile
	_ = json.Unmarshal(raw, &out)
	return out
}
