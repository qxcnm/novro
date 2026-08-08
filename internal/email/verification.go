package email

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/novro-gateway/novro/internal/user"
)

var (
	ErrRateLimited = errors.New("verification code requested too recently")
	ErrInvalidCode = errors.New("invalid verification code")
	ErrExpired     = errors.New("verification code expired")
)

type VerificationStore interface {
	Issue(context.Context, string, string, time.Time, time.Time) error
	DeleteIssue(context.Context, string, string) error
	Consume(context.Context, string, string, time.Time) error
}

type VerificationService struct {
	store     VerificationStore
	mailer    Mailer
	secret    []byte
	now       func() time.Time
	generate  func() (string, error)
	expiresIn time.Duration
}

func NewVerificationService(store VerificationStore, mailer Mailer, secret string) (*VerificationService, error) {
	if store == nil || mailer == nil || len([]byte(secret)) < 32 {
		return nil, errors.New("email verification requires a store, mailer, and 32-byte secret")
	}
	return &VerificationService{store: store, mailer: mailer, secret: []byte(secret), now: func() time.Time { return time.Now().UTC() }, generate: newCode, expiresIn: 10 * time.Minute}, nil
}

func (s *VerificationService) Send(ctx context.Context, rawEmail string) error {
	email, ok := user.NormalizeEmail(rawEmail)
	if !ok {
		return user.ErrInvalidInput
	}
	now := s.now()
	code, err := s.generate()
	if err != nil {
		return fmt.Errorf("generate email verification code: %w", err)
	}
	if err := s.store.Issue(ctx, email, s.hash(email, code), now.Add(s.expiresIn), now); err != nil {
		return err
	}
	if err := s.mailer.SendVerificationCode(ctx, email, code); err != nil {
		_ = s.store.DeleteIssue(ctx, email, s.hash(email, code))
		return fmt.Errorf("send email verification code: %w", err)
	}
	return nil
}

func (s *VerificationService) Verify(ctx context.Context, rawEmail, code string) error {
	email, ok := user.NormalizeEmail(rawEmail)
	if !ok || len(strings.TrimSpace(code)) != 6 {
		return ErrInvalidCode
	}
	if err := s.store.Consume(ctx, email, s.hash(email, strings.TrimSpace(code)), s.now()); err != nil {
		return err
	}
	return nil
}

func (s *VerificationService) hash(email, code string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(email + "\x00" + code))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func newCode() (string, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(random[:])%1000000), nil
}
