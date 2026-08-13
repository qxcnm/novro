package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxActiveKeysPerUser = 10

type Store interface {
	/**
	 * Create 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @param arg4 类型为 string 的接口输入参数。
	 * @param arg5 类型为 string 的接口输入参数。
	 * @param arg6 类型为 string 的接口输入参数。
	 * @param arg7 类型为 string 的接口输入参数。
	 * @param arg8 类型为 int 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Create(context.Context, uuid.UUID, uuid.UUID, string, string, string, string, int) (Record, error)
	/**
	 * ListByUser 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ListByUser(context.Context, uuid.UUID) ([]Record, error)
	/**
	 * GetByUser 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	GetByUser(context.Context, uuid.UUID, uuid.UUID) (Record, error)
	/**
	 * RevokeByUser 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @param arg4 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	RevokeByUser(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	/**
	 * ListAll 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 ListFilter 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ListAll(context.Context, ListFilter) (Page, error)
	/**
	 * Revoke 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Revoke(context.Context, uuid.UUID, time.Time) error
	/**
	 * AuthenticateHash 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @param arg3 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	AuthenticateHash(context.Context, string, time.Time) (Actor, error)
}

type SecretCipher interface {
	/**
	 * Encrypt 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Encrypt(string) (string, error)
	/**
	 * Decrypt 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Decrypt(string) (string, error)
}

/**
 * Authenticate 用于校验用户凭据并建立登录会话。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param token 用于认证或继续操作的令牌。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Authenticate(ctx context.Context, token string) (Actor, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "nvr_") || len(token) < 40 || len(token) > 128 {
		return Actor{}, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(token))
	return s.store.AuthenticateHash(ctx, hex.EncodeToString(digest[:]), s.now())
}

type Service struct {
	store         Store
	cipher        SecretCipher
	now           func() time.Time
	generateToken func() (string, error)
}

type CreateResult struct {
	APIKey Record `json:"api_key"`
	Key    string `json:"key"`
}

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @param cipher 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store, cipher SecretCipher) *Service {
	return &Service{store: store, cipher: cipher, now: func() time.Time { return time.Now().UTC() }, generateToken: newToken}
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @param name 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Create(ctx context.Context, userID, billingGroupID uuid.UUID, name string) (CreateResult, error) {
	name = strings.TrimSpace(name)
	if userID == uuid.Nil || billingGroupID == uuid.Nil || name == "" || utf8.RuneCountInString(name) > 64 {
		return CreateResult{}, ErrInvalidInput
	}
	token, err := s.generateToken()
	if err != nil {
		return CreateResult{}, fmt.Errorf("generate API key: %w", err)
	}
	if s.cipher == nil {
		return CreateResult{}, fmt.Errorf("API key cipher is not configured")
	}
	encryptedSecret, err := s.cipher.Encrypt(token)
	if err != nil {
		return CreateResult{}, fmt.Errorf("encrypt API key secret: %w", err)
	}
	digest := sha256.Sum256([]byte(token))
	prefix := token[:12]
	record, err := s.store.Create(ctx, userID, billingGroupID, name, prefix, hex.EncodeToString(digest[:]), encryptedSecret, maxActiveKeysPerUser)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{APIKey: record, Key: token}, nil
}

/**
 * RevealForUser 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) RevealForUser(ctx context.Context, userID, id uuid.UUID) (string, error) {
	if userID == uuid.Nil || id == uuid.Nil {
		return "", ErrInvalidInput
	}
	if s.cipher == nil {
		return "", fmt.Errorf("API key cipher is not configured")
	}
	record, err := s.store.GetByUser(ctx, userID, id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(record.KeySecretCiphertext) == "" {
		return "", ErrSecretUnavailable
	}
	secret, err := s.cipher.Decrypt(record.KeySecretCiphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt API key secret: %w", err)
	}
	return secret, nil
}

/**
 * ListForUser 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Record, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.store.ListByUser(ctx, userID)
}

/**
 * RevokeForUser 用于删除、撤销或释放指定资源。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) RevokeForUser(ctx context.Context, userID, id uuid.UUID) error {
	if userID == uuid.Nil || id == uuid.Nil {
		return ErrInvalidInput
	}
	return s.store.RevokeByUser(ctx, userID, id, s.now())
}

/**
 * ListAll 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) ListAll(ctx context.Context, filter ListFilter) (Page, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusRevoked {
		return Page{}, ErrInvalidInput
	}
	if filter.Offset < 0 {
		return Page{}, ErrInvalidInput
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		return Page{}, ErrInvalidInput
	}
	return s.store.ListAll(ctx, filter)
}

/**
 * Revoke 用于删除、撤销或释放指定资源。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Revoke(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	return s.store.Revoke(ctx, id, s.now())
}

/**
 * newToken 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func newToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "nvr_" + base64.RawURLEncoding.EncodeToString(random), nil
}
