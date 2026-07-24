package webhook

import (
	"testing"
	"time"
)

func testDefinition() Definition {
	return Definition{
		Version:  DefinitionVersion,
		Consumer: "test",
		Source:   "urn:yueli:test:sender",
		EventTypes: []EventTypeDefinition{{
			Type: "com.yueli.test.created.v1", MaxDataBytes: 4096,
		}},
		InboundSources: []InboundSourceDefinition{{
			Key: "test.sender", ExpectedSource: "urn:yueli:test:sender",
			AllowedTypes: []EventType{"com.yueli.test.created.v1"},
			Secret:       "inbound.test", TimestampWindow: 5 * time.Minute,
		}},
	}
}

func TestDefinitionDigestIsCanonical(t *testing.T) {
	first, err := Compile(testDefinition())
	if err != nil {
		t.Fatal(err)
	}
	secondDefinition := testDefinition()
	secondDefinition.InboundSources[0].AllowedTypes = append([]EventType(nil), secondDefinition.InboundSources[0].AllowedTypes...)
	second, err := Compile(secondDefinition)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() != second.Digest() {
		t.Fatalf("digests differ: %s %s", first.Digest(), second.Digest())
	}
}

func TestDefinitionRejectsUnboundedOrUnknownInbound(t *testing.T) {
	definition := testDefinition()
	definition.InboundSources[0].AllowedTypes = []EventType{"com.yueli.unknown.v1"}
	if _, err := Compile(definition); !IsCode(err, ErrorInvalidDefinition) {
		t.Fatalf("unknown inbound type err=%v", err)
	}
	definition = testDefinition()
	definition.Retry.RequestTimeout = time.Minute
	if _, err := Compile(definition); !IsCode(err, ErrorInvalidDefinition) {
		t.Fatalf("unbounded timeout err=%v", err)
	}
}
