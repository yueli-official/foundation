package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderWebhookID        = "Webhook-Id"
	HeaderWebhookTimestamp = "Webhook-Timestamp"
	HeaderWebhookSignature = "Webhook-Signature"
	HeaderDeliveryID       = "Webhook-Delivery-Id"
	CloudEventsContentType = "application/cloudevents+json"
)

type cloudEvent struct {
	SpecVersion     string          `json:"specversion"`
	ID              EventID         `json:"id"`
	Source          string          `json:"source"`
	Type            EventType       `json:"type"`
	Subject         string          `json:"subject,omitempty"`
	Time            time.Time       `json:"time"`
	DataContentType string          `json:"datacontenttype"`
	Data            json.RawMessage `json:"data"`
	TraceParent     string          `json:"traceparent,omitempty"`
}

func encodeCloudEvent(catalog *Catalog, id EventID, command EventCommand) ([]byte, error) {
	return json.Marshal(cloudEvent{
		SpecVersion: "1.0", ID: id, Source: catalog.source, Type: command.Type,
		Subject: command.Subject, Time: command.OccurredAt.UTC(),
		DataContentType: "application/json", Data: command.Data, TraceParent: command.TraceParent,
	})
}

func SignV1(id string, timestamp time.Time, body, secret []byte) string {
	seconds := strconv.FormatInt(timestamp.UTC().Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(id))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write([]byte(seconds))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func VerifyV1(id, timestamp string, body []byte, signatures []string, secrets []SecretMaterial) (SecretRevision, error) {
	if id == "" || strings.Contains(id, ".") || timestamp == "" {
		return "", invalid(ErrorSignatureMissing, "headers", "required Standard Webhooks headers are missing")
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return "", invalid(ErrorSignatureInvalid, "webhook_timestamp", "is invalid")
	}
	instant := time.Unix(seconds, 0).UTC()
	for _, secret := range secrets {
		expected := SignV1(id, instant, body, secret.Value)
		for _, header := range signatures {
			for _, candidate := range strings.Fields(header) {
				if hmac.Equal([]byte(expected), []byte(candidate)) {
					return secret.Revision, nil
				}
			}
		}
	}
	return "", invalid(ErrorSignatureInvalid, "webhook_signature", "does not match a trusted secret")
}

func NewID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", unavailable("generate id", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	text := hex.EncodeToString(value[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", text[:8], text[8:12], text[12:16], text[16:20], text[20:]), nil
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
