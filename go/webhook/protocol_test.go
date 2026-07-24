package webhook

import (
	"net/http"
	"testing"
	"time"
)

func TestStandardWebhookV1VectorAndRawBody(t *testing.T) {
	id := "msg_2KWPBgLlAfxdpx2AI54pPJ85f4W"
	at := time.Unix(1674087231, 0).UTC()
	body := []byte(`{"type":"example.event","timestamp":"2022-11-03T20:26:10.344522Z","data":{"id":"1f81eb52-5198-4599-803e-771906343485"}}`)
	secret := []byte("test-secret-value-32-bytes-long!!")
	signature := SignV1(id, at, body, secret)
	const expected = "v1,G7dPLwLQ9WifGSTe2KyIsT/vUxrYd3GH8WqXY7gliB0="
	if signature != expected {
		t.Fatalf("signature = %q", signature)
	}
	revision, err := VerifyV1(id, "1674087231", body, []string{signature}, []SecretMaterial{{
		Revision: "current", Value: secret,
	}})
	if err != nil || revision != "current" {
		t.Fatalf("verify revision=%q err=%v", revision, err)
	}
	if _, err := VerifyV1(id, "1674087231", append(body, ' '), []string{signature}, []SecretMaterial{{
		Revision: "current", Value: secret,
	}}); !IsCode(err, ErrorSignatureInvalid) {
		t.Fatalf("modified raw body err=%v", err)
	}
}

func TestMultipleSignaturesSupportRotation(t *testing.T) {
	id := "event-1"
	at := time.Unix(1700000000, 0)
	body := []byte(`{"ok":true}`)
	current := SecretMaterial{Revision: "r2", Value: []byte("current-secret-value-32-bytes!!!!")}
	previous := SecretMaterial{Revision: "r1", Value: []byte("previous-secret-value-32-bytes!!!")}
	headers := http.Header{}
	headers.Add(HeaderWebhookSignature, SignV1(id, at, body, current.Value))
	headers.Add(HeaderWebhookSignature, SignV1(id, at, body, previous.Value))
	revision, err := VerifyV1(id, "1700000000", body, headers.Values(HeaderWebhookSignature), []SecretMaterial{previous})
	if err != nil || revision != "r1" {
		t.Fatalf("previous rotation signature revision=%q err=%v", revision, err)
	}
}
