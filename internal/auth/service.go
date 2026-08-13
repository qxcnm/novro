package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/novro-gateway/novro/internal/user"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrOIDCNotProvisioned = errors.New("OIDC user is not provisioned")
)

type PasswordManager interface {
	/**
	 * Hash 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Hash(string) (string, error)
	/**
	 * Verify 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 string 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Verify(string, string) bool
}

type LoginUser struct {
	User         user.Record
	PasswordHash *string
}

type Store interface {
	/**
	 * FindUserByUsername 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	FindUserByUsername(context.Context, string) (LoginUser, error)
	/**
	 * FindOrCreateOIDCUser 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 OIDCUser 的接口输入参数。
	 * @param arg3 类型为 bool 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	FindOrCreateOIDCUser(context.Context, OIDCUser, bool) (user.Record, error)
	/**
	 * CreateSession 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @param arg4 类型为 time.Time 的接口输入参数。
	 * @param arg5 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	CreateSession(context.Context, uuid.UUID, string, time.Time, time.Time) error
	/**
	 * FindUserBySession 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @param arg3 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	FindUserBySession(context.Context, string, time.Time) (user.Record, error)
	/**
	 * RevokeSession 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @param arg3 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	RevokeSession(context.Context, string, time.Time) error
}

type OIDCUser struct {
	Issuer            string
	Subject           string
	Email             string
	PreferredUsername string
	DisplayName       string
}

type Service struct {
	store         Store
	passwords     PasswordManager
	secret        string
	ttl           time.Duration
	dummyHash     string
	now           func() time.Time
	generateToken func() (string, error)
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      user.Record
}

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @param passwords 本次操作需要使用的输入参数。
 * @param secret 本次操作需要使用的输入参数。
 * @param ttl 本次操作使用的数值参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store, passwords PasswordManager, secret string, ttl time.Duration) (*Service, error) {
	dummyHash, err := passwords.Hash("novro-dummy-password-value1")
	if err != nil {
		return nil, fmt.Errorf("create authentication comparison hash: %w", err)
	}
	return &Service{
		store:         store,
		passwords:     passwords,
		secret:        secret,
		ttl:           ttl,
		dummyHash:     dummyHash,
		now:           func() time.Time { return time.Now().UTC() },
		generateToken: newSessionToken,
	}, nil
}

/**
 * Login 用于校验用户凭据并建立登录会话。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param identifier 本次操作需要使用的输入参数。
 * @param plainTextPassword 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Login(ctx context.Context, identifier, plainTextPassword string) (LoginResult, error) {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	loginUser, err := s.store.FindUserByUsername(ctx, identifier)
	if err != nil {
		_ = s.passwords.Verify(s.dummyHash, plainTextPassword)
		if errors.Is(err, user.ErrNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, fmt.Errorf("find login user: %w", err)
	}
	passwordValid := false
	if loginUser.PasswordHash != nil {
		passwordValid = s.passwords.Verify(*loginUser.PasswordHash, plainTextPassword)
	} else {
		_ = s.passwords.Verify(s.dummyHash, plainTextPassword)
	}
	if loginUser.User.Status != user.StatusActive || !passwordValid {
		return LoginResult{}, ErrInvalidCredentials
	}

	return s.createLoginSession(ctx, loginUser.User)
}

/**
 * LoginOIDC 用于校验用户凭据并建立登录会话。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param identity 本次操作需要使用的输入参数。
 * @param autoRegister 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) LoginOIDC(ctx context.Context, identity OIDCUser, autoRegister bool) (LoginResult, error) {
	record, err := s.store.FindOrCreateOIDCUser(ctx, identity, autoRegister)
	if err != nil {
		return LoginResult{}, err
	}
	if record.Status != user.StatusActive {
		return LoginResult{}, ErrUnauthenticated
	}
	return s.createLoginSession(ctx, record)
}

/**
 * createLoginSession 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param record 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) createLoginSession(ctx context.Context, record user.Record) (LoginResult, error) {
	token, err := s.generateToken()
	if err != nil {
		return LoginResult{}, err
	}
	now := s.now()
	expiresAt := now.Add(s.ttl)
	if err := s.store.CreateSession(ctx, record.ID, hashSessionToken(s.secret, token), expiresAt, now); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, fmt.Errorf("create login session: %w", err)
	}
	record.LastLoginAt = &now
	return LoginResult{Token: token, ExpiresAt: expiresAt, User: record}, nil
}

/**
 * Authenticate 用于校验用户凭据并建立登录会话。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param token 用于认证或继续操作的令牌。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Authenticate(ctx context.Context, token string) (user.Record, error) {
	if !strings.HasPrefix(token, "nvs_") || len(token) < 40 || len(token) > 128 {
		return user.Record{}, ErrUnauthenticated
	}
	record, err := s.store.FindUserBySession(ctx, hashSessionToken(s.secret, token), s.now())
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return user.Record{}, ErrUnauthenticated
		}
		return user.Record{}, fmt.Errorf("authenticate session: %w", err)
	}
	return record, nil
}

/**
 * Logout 用于撤销当前用户的登录会话。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param token 用于认证或继续操作的令牌。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.store.RevokeSession(ctx, hashSessionToken(s.secret, token), s.now()); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
