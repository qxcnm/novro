package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeStore struct {
	createdName    string
	createdGroupID uuid.UUID
	createdPrefix  string
	createdHash    string
	createdSecret  string
	createdLimit   int
	listFilter     ListFilter
	lookupUserID   uuid.UUID
	lookupID       uuid.UUID
	revokedUserID  uuid.UUID
	revokedID      uuid.UUID
	err            error
	authHash       string
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param userID 目标用户的唯一标识。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @param name 用于标识或筛选目标的文本值。
 * @param prefix 本次操作需要使用的输入参数。
 * @param hash 控制对应行为是否启用的布尔值。
 * @param secret 本次操作需要使用的输入参数。
 * @param limit 本次操作使用的数值参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Create(_ context.Context, userID, billingGroupID uuid.UUID, name, prefix, hash, secret string, limit int) (Record, error) {
	f.createdName, f.createdPrefix, f.createdHash, f.createdSecret, f.createdLimit = name, prefix, hash, secret, limit
	f.createdGroupID = billingGroupID
	if f.err != nil {
		return Record{}, f.err
	}
	return Record{ID: uuid.New(), UserID: userID, BillingGroupID: billingGroupID, Name: name, KeyPrefix: prefix, CanCopySecret: secret != "", KeySecretCiphertext: secret, Status: StatusActive}, nil
}

/**
 * ListByUser 用于筛选并返回数据列表。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) ListByUser(context.Context, uuid.UUID) ([]Record, error) { return nil, f.err }

/**
 * GetByUser 用于查询并返回所需的数据。
 * @param userID 目标用户的唯一标识。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) GetByUser(_ context.Context, userID, id uuid.UUID) (Record, error) {
	f.lookupUserID, f.lookupID = userID, id
	if f.err != nil {
		return Record{}, f.err
	}
	if f.createdSecret == "" {
		return Record{}, ErrSecretUnavailable
	}
	return Record{ID: id, UserID: userID, KeySecretCiphertext: f.createdSecret}, nil
}

/**
 * RevokeByUser 用于删除、撤销或释放指定资源。
 * @param userID 目标用户的唯一标识。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) RevokeByUser(_ context.Context, userID, id uuid.UUID, _ time.Time) error {
	f.revokedUserID, f.revokedID = userID, id
	return f.err
}

/**
 * ListAll 用于筛选并返回数据列表。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) ListAll(_ context.Context, filter ListFilter) (Page, error) {
	f.listFilter = filter
	return Page{Limit: filter.Limit}, f.err
}

/**
 * Revoke 用于删除、撤销或释放指定资源。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Revoke(_ context.Context, id uuid.UUID, _ time.Time) error {
	f.revokedID = id
	return f.err
}

/**
 * AuthenticateHash 用于校验用户凭据并建立登录会话。
 * @param hash 控制对应行为是否启用的布尔值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) AuthenticateHash(_ context.Context, hash string, _ time.Time) (Actor, error) {
	f.authHash = hash
	return Actor{}, f.err
}

/**
 * TestCreateReturnsSecretOnceAndStoresOnlyHash 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestCreateReturnsSecretOnceAndStoresOnlyHash(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeCipher{})
	service.generateToken = func() (string, error) { return "nvr_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", nil }
	groupID := uuid.New()
	result, err := service.Create(context.Background(), uuid.New(), groupID, "  Production  ")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if result.Key == "" || store.createdName != "Production" || store.createdGroupID != groupID || store.createdPrefix != result.Key[:12] || store.createdLimit != maxActiveKeysPerUser {
		t.Fatalf("unexpected create result=%+v store=%+v", result, store)
	}
	if store.createdHash == result.Key || len(store.createdHash) != 64 || strings.Contains(store.createdHash, "nvr_") {
		t.Fatalf("plaintext API key reached persistence: %q", store.createdHash)
	}
	if store.createdSecret != "enc:"+result.Key {
		t.Fatalf("expected encrypted secret, got %q", store.createdSecret)
	}
}

/**
 * TestCreateValidatesNameAndPreservesLimitError 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestCreateValidatesNameAndPreservesLimitError(t *testing.T) {
	service := NewService(&fakeStore{}, fakeCipher{})
	if _, err := service.Create(context.Background(), uuid.New(), uuid.New(), " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid name, got %v", err)
	}
	service = NewService(&fakeStore{err: ErrLimitReached}, fakeCipher{})
	if _, err := service.Create(context.Background(), uuid.New(), uuid.New(), "Test"); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("expected limit error, got %v", err)
	}
}

/**
 * TestAuthenticateHashesTokenBeforePersistence 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestAuthenticateHashesTokenBeforePersistence(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeCipher{})
	if _, err := service.Authenticate(context.Background(), "nvr_test-token-value-12345678901234567890"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if len(store.authHash) != 64 || strings.Contains(store.authHash, "nvr_") {
		t.Fatalf("plaintext token reached persistence: %q", store.authHash)
	}
	if _, err := service.Authenticate(context.Background(), "invalid"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected invalid token rejection, got %v", err)
	}
}

/**
 * TestListAndRevokeValidateScope 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestListAndRevokeValidateScope(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeCipher{})
	page, err := service.ListAll(context.Background(), ListFilter{})
	if err != nil || page.Limit != 50 || store.listFilter.Limit != 50 {
		t.Fatalf("default list: page=%+v filter=%+v err=%v", page, store.listFilter, err)
	}
	if _, err := service.ListAll(context.Background(), ListFilter{Status: "unknown"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid status, got %v", err)
	}
	userID, id := uuid.New(), uuid.New()
	if err := service.RevokeForUser(context.Background(), userID, id); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if store.revokedUserID != userID || store.revokedID != id {
		t.Fatalf("revoke escaped user scope: %+v", store)
	}
}

/**
 * TestRevealForUserDecryptsSecret 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestRevealForUserDecryptsSecret(t *testing.T) {
	store := &fakeStore{createdSecret: "enc:nvr_secret-value"}
	service := NewService(store, fakeCipher{})
	secret, err := service.RevealForUser(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if secret != "nvr_secret-value" {
		t.Fatalf("unexpected secret: %q", secret)
	}
	if store.lookupUserID == uuid.Nil || store.lookupID == uuid.Nil {
		t.Fatalf("expected lookup scope to be enforced: %+v", store)
	}
}

/**
 * TestRevealForUserRejectsMissingSecret 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestRevealForUserRejectsMissingSecret(t *testing.T) {
	service := NewService(&fakeStore{}, fakeCipher{})
	if _, err := service.RevealForUser(context.Background(), uuid.New(), uuid.New()); !errors.Is(err, ErrSecretUnavailable) {
		t.Fatalf("expected missing secret error, got %v", err)
	}
}

type fakeCipher struct{}

/**
 * Encrypt 用于对敏感数据执行安全转换。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakeCipher) Encrypt(value string) (string, error) { return "enc:" + value, nil }

/**
 * Decrypt 用于解密并返回受保护的数据。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakeCipher) Decrypt(value string) (string, error) {
	return strings.TrimPrefix(value, "enc:"), nil
}
