package password

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerify(t *testing.T) {
	hasher := Hasher{Cost: bcrypt.MinCost}
	hash, err := hasher.Hash("a-long-test-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "a-long-test-password" || !hasher.Verify(hash, "a-long-test-password") {
		t.Fatal("password did not round-trip through bcrypt")
	}
	if hasher.Verify(hash, "different-password") {
		t.Fatal("wrong password was accepted")
	}
}

func TestValidatePasswordBounds(t *testing.T) {
	for _, value := range []string{"short", strings.Repeat("x", MaxPasswordBytes+1)} {
		if err := Validate(value); !errors.Is(err, ErrInvalidPassword) {
			t.Fatalf("expected invalid password for %d bytes, got %v", len(value), err)
		}
	}
	if err := Validate(strings.Repeat("密", 24)); err != nil {
		t.Fatalf("expected 72-byte UTF-8 password to be valid: %v", err)
	}
}
