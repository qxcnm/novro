package email

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	hash, email string
	expires     time.Time
	created     time.Time
	consumed    bool
	rateLimited bool
}

func (f *fakeStore) Issue(_ context.Context, email, hash string, expiresAt, now time.Time) error {
	if f.rateLimited {
		return ErrRateLimited
	}
	f.email, f.hash, f.expires, f.created = email, hash, expiresAt, now
	return nil
}
func (f *fakeStore) Consume(_ context.Context, email, hash string, now time.Time) error {
	if email != f.email || hash != f.hash {
		return ErrInvalidCode
	}
	if !f.expires.After(now) {
		return ErrExpired
	}
	if f.consumed {
		return ErrInvalidCode
	}
	f.consumed = true
	return nil
}
func (f *fakeStore) DeleteIssue(context.Context, string, string) error { return nil }

type fakeMailer struct {
	recipient, code string
	err             error
}

func (f *fakeMailer) SendVerificationCode(_ context.Context, recipient, code string) error {
	f.recipient, f.code = recipient, code
	return f.err
}

func TestVerificationServiceNormalizesAndConsumesOnce(t *testing.T) {
	store, mailer := &fakeStore{}, &fakeMailer{}
	service, err := NewVerificationService(store, mailer, "01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.generate = func() (string, error) { return "123456", nil }
	if err := service.Send(context.Background(), " User@Example.COM "); err != nil {
		t.Fatal(err)
	}
	if mailer.recipient != "user@example.com" || mailer.code != "123456" {
		t.Fatalf("unexpected message: %+v", mailer)
	}
	if err := service.Verify(context.Background(), "user@example.com", "123456"); err != nil {
		t.Fatal(err)
	}
	if err := service.Verify(context.Background(), "user@example.com", "123456"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected one-time code error, got %v", err)
	}
}

func TestVerificationServiceRejectsInvalidAndExpiredCodes(t *testing.T) {
	store, mailer := &fakeStore{}, &fakeMailer{}
	service, _ := NewVerificationService(store, mailer, "01234567890123456789012345678901")
	service.now = func() time.Time { return time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC) }
	if err := service.Verify(context.Background(), "bad", "123456"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected invalid code, got %v", err)
	}
	store.email, store.hash, store.expires = "user@example.com", service.hash("user@example.com", "123456"), service.now().Add(-time.Second)
	if err := service.Verify(context.Background(), "user@example.com", "123456"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected expired code, got %v", err)
	}
}
