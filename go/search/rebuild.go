package search

import "time"

type RebuildPhase string

const (
	RebuildBuilding  RebuildPhase = "building"
	RebuildActive    RebuildPhase = "active"
	RebuildAbandoned RebuildPhase = "abandoned"
)

type StartRebuild struct {
	RequestID string
}

type RebuildBatch struct {
	Generation GenerationID
	Batch      Batch
	Checkpoint string
}

type FinishRebuild struct {
	Generation        GenerationID
	FinalCheckpoint   string
	ExpectedDocuments uint64
}

type RebuildState struct {
	Generation GenerationID `json:"generation"`
	Phase      RebuildPhase `json:"phase"`
	Checkpoint string       `json:"checkpoint"`
	Documents  uint64       `json:"documents"`
	StartedAt  time.Time    `json:"startedAt"`
	UpdatedAt  time.Time    `json:"updatedAt"`
}
