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

/**
 * Issue 封装该名称对应的业务处理逻辑。
 * @param email 本次操作需要使用的输入参数。
 * @param hash 控制对应行为是否启用的布尔值。
 * @param expiresAt 本次操作需要使用的输入参数。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Issue(_ context.Context, email, hash string, expiresAt, now time.Time) error {
	if f.rateLimited {
		return ErrRateLimited
	}
	f.email, f.hash, f.expires, f.created = email, hash, expiresAt, now
	return nil
}

/**
 * Consume 封装该名称对应的业务处理逻辑。
 * @param email 本次操作需要使用的输入参数。
 * @param hash 控制对应行为是否启用的布尔值。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * DeleteIssue 用于删除、撤销或释放指定资源。
 * @param string 本次操作需要使用的输入参数。
 * @param string 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) DeleteIssue(context.Context, string, string) error { return nil }

type fakeMailer struct {
	recipient, code string
	err             error
}

/**
 * SendVerificationCode 用于发送对应消息或请求。
 * @param recipient 本次操作需要使用的输入参数。
 * @param code 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeMailer) SendVerificationCode(_ context.Context, recipient, code string) error {
	f.recipient, f.code = recipient, code
	return f.err
}

/**
 * TestVerificationServiceNormalizesAndConsumesOnce 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestVerificationServiceRejectsInvalidAndExpiredCodes 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
