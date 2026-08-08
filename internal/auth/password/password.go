package password

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultCost      = 12
	MinPasswordBytes = 8
	MaxPasswordBytes = 72
)

var ErrInvalidPassword = errors.New("password must be 8 to 72 bytes, valid UTF-8, and contain an English letter and a digit")

type Hasher struct {
	Cost int
}

func (h Hasher) Hash(plainText string) (string, error) {
	if err := Validate(plainText); err != nil {
		return "", err
	}
	cost := h.Cost
	if cost == 0 {
		cost = DefaultCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plainText), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (Hasher) Verify(hash, plainText string) bool {
	if hash == "" || !utf8.ValidString(plainText) || len([]byte(plainText)) > MaxPasswordBytes {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plainText)) == nil
}

func Validate(plainText string) error {
	length := len([]byte(plainText))
	if !utf8.ValidString(plainText) || length < MinPasswordBytes || length > MaxPasswordBytes {
		return ErrInvalidPassword
	}
	var hasEnglishLetter, hasDigit bool
	for _, character := range plainText {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') {
			hasEnglishLetter = true
		}
		if character >= '0' && character <= '9' {
			hasDigit = true
		}
	}
	if !hasEnglishLetter || !hasDigit {
		return ErrInvalidPassword
	}
	return nil
}
