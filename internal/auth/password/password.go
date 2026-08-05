package password

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultCost      = 12
	MinPasswordBytes = 12
	MaxPasswordBytes = 72
)

var ErrInvalidPassword = errors.New("password must be 12 to 72 bytes and valid UTF-8")

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
	return nil
}
