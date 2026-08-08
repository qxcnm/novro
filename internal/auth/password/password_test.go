package password

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerify(t *testing.T) {
	hasher := Hasher{Cost: bcrypt.MinCost}
	hash, err := hasher.Hash("a-long-test-password1")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "a-long-test-password1" || !hasher.Verify(hash, "a-long-test-password1") {
		t.Fatal("password did not round-trip through bcrypt")
	}
	if hasher.Verify(hash, "different-password") {
		t.Fatal("wrong password was accepted")
	}
}

func TestValidatePasswordBounds(t *testing.T) {
	for _, value := range []string{"short1", "12345678", "abcdefgh", strings.Repeat("a", MaxPasswordBytes)} {
		if err := Validate(value); !errors.Is(err, ErrInvalidPassword) {
			t.Fatalf("expected invalid password for %d bytes, got %v", len(value), err)
		}
	}
	if err := Validate(strings.Repeat("密", 23) + "ab1"); err != nil {
		t.Fatalf("expected 72-byte UTF-8 password to be valid: %v", err)
	}
	if err := Validate("Abcdefg1"); err != nil {
		t.Fatalf("expected 8-byte mixed password to be valid: %v", err)
	}
	if err := Validate("TestPass1"); err != nil {
		t.Fatalf("expected bootstrap password to satisfy the policy: %v", err)
	}
	if err := Validate(strings.Repeat("a", MaxPasswordBytes-1) + "1"); err != nil {
		t.Fatalf("expected maximum-length mixed password to be valid: %v", err)
	}
	if err := Validate(strings.Repeat("a", MaxPasswordBytes) + "1"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected overlength password to be invalid, got %v", err)
	}
}
