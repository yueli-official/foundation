package privacygoframe

import "testing"

func TestNewOwnerHandlerRejectsNilHost(t *testing.T) {
	if _, err := NewOwnerHandler(nil); err == nil {
		t.Fatal("NewOwnerHandler() accepted nil host")
	}
}
