package abuse

import (
	"encoding/hex"
	"net/netip"
	"time"
)

type ActionKey string
type PolicyID string
type SlotKey string
type OutcomeKey string
type AttemptID string
type ChallengeKind string

const (
	SlotNetwork SlotKey = "network"
	SlotActor   SlotKey = "actor"
	SlotTarget  SlotKey = "target"
)

type Disposition string

const (
	DispositionUnknown   Disposition = ""
	DispositionAllow     Disposition = "allow"
	DispositionChallenge Disposition = "challenge"
	DispositionReject    Disposition = "reject"
)

type Signals struct {
	Network netip.Prefix
	Actor   string
	Target  string
	Extra   []Signal
}

type Signal struct {
	Slot      SlotKey
	Canonical string
}

type Proof struct {
	Kind  ChallengeKind
	Token string
}

type Input struct {
	ID      AttemptID
	Signals Signals
	Proof   *Proof
}

type Receipt struct {
	id          AttemptID
	action      ActionKey
	fingerprint [32]byte
}

func (receipt Receipt) ID() AttemptID {
	return receipt.id
}

func (receipt Receipt) Action() ActionKey {
	return receipt.action
}

func (receipt Receipt) IsZero() bool {
	return receipt.id == ""
}

type Admission struct {
	Receipt     Receipt
	Disposition Disposition
	RetryAt     time.Time
	Challenge   *Challenge
	Replay      bool
	Findings    []Finding
}

type Challenge struct {
	Kind           ChallengeKind
	ContinuationID AttemptID
}

type Finding struct {
	Policy      PolicyID
	Disposition Disposition
	RetryAt     time.Time
	Used        int64
	Capacity    int64
}

type VerificationRequest struct {
	VerificationID string
	Token          string
	ExpectedAction string
	AllowedHosts   []string
}

type VerificationStatus string

const (
	VerificationAccepted VerificationStatus = "accepted"
	VerificationRejected VerificationStatus = "rejected"
)

type Verification struct {
	Status   VerificationStatus
	SolvedAt time.Time
	Action   string
	Hostname string
	Codes    []string
}

type ActionView struct {
	Key                ActionKey
	RequiredSlots      []SlotKey
	ResolutionRequired bool
}

type InspectQuery struct {
	Action  ActionKey
	Signals Signals
}

type MeterSnapshot struct {
	Policy     PolicyID
	Slot       SlotKey
	SubjectRef string
	Used       int64
	Capacity   int64
	ExpiresAt  time.Time
}

type Inspection struct {
	Action ActionKey
	Meters []MeterSnapshot
}

type ResetCommand struct {
	Action  ActionKey
	Signals Signals
	Reason  string
}

type ResetResult struct {
	MetersReset   int64
	EventsRemoved int64
}

type PruneCommand struct {
	Before time.Time
	Limit  int
}

type PruneResult struct {
	StatesRemoved    int64
	ReceiptsRemoved  int64
	EventsRemoved    int64
	PendingFinalized int64
}

type subjectKey [32]byte

func (key subjectKey) hex() string {
	return hex.EncodeToString(key[:])
}
