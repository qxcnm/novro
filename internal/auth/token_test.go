package auth

import (
	"strings"
	"testing"
)

func TestSessionTokensAreRandomAndKeyed(t *testing.T) {
	first, err := newSessionToken()
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	second, err := newSessionToken()
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if first == second || !strings.HasPrefix(first, "nvs_") {
		t.Fatalf("unexpected tokens: %q %q", first, second)
	}
	firstHash := hashSessionToken("secret-one", first)
	if len(firstHash) != 64 || firstHash == hashSessionToken("secret-two", first) {
		t.Fatal("session hash is not a keyed SHA-256 value")
	}
}
