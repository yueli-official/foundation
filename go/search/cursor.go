package search

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
)

type cursorPayload struct {
	Version    int          `json:"v"`
	Generation GenerationID `json:"g"`
	Query      string       `json:"q"`
	Sort       SortKind     `json:"s"`
	ScoreBits  uint32       `json:"r,omitempty"`
	SortAt     int64        `json:"t"`
	Kind       DocumentKind `json:"k"`
	ID         DocumentID   `json:"i"`
	IssuedAt   int64        `json:"a"`
	Checksum   string       `json:"c,omitempty"`
}

func encodeCursor(catalogDigest string, value cursorPayload) Cursor {
	value.Version = 1
	value.Checksum = ""
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(append([]byte(catalogDigest), raw...))
	value.Checksum = hex.EncodeToString(sum[:])
	raw, _ = json.Marshal(value)
	return Cursor(base64.RawURLEncoding.EncodeToString(raw))
}

func decodeCursor(catalogDigest string, raw Cursor) (cursorPayload, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(string(raw))
	if err != nil {
		return cursorPayload{}, invalidCursor()
	}
	var value cursorPayload
	if json.Unmarshal(decoded, &value) != nil || value.Version != 1 || value.Generation == "" {
		return cursorPayload{}, invalidCursor()
	}
	checksum := value.Checksum
	value.Checksum = ""
	canonical, _ := json.Marshal(value)
	sum := sha256.Sum256(append([]byte(catalogDigest), canonical...))
	if checksum != hex.EncodeToString(sum[:]) {
		return cursorPayload{}, invalidCursor()
	}
	value.Checksum = checksum
	return value, nil
}

func invalidCursor() error {
	return &Error{Kind: ErrorInvalidCursor, Field: "cursor", Message: "is invalid or belongs to another query"}
}

func cursorScore(value cursorPayload) float32 { return math.Float32frombits(value.ScoreBits) }
