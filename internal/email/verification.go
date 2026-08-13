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
	/**
	 * Issue 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @param arg4 类型为 time.Time 的接口输入参数。
	 * @param arg5 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Issue(context.Context, string, string, time.Time, time.Time) error
	/**
	 * DeleteIssue 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	DeleteIssue(context.Context, string, string) error
	/**
	 * Consume 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @param arg4 类型为 time.Time 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
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

/**
 * NewVerificationService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @param mailer 本次操作需要使用的输入参数。
 * @param secret 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewVerificationService(store VerificationStore, mailer Mailer, secret string) (*VerificationService, error) {
	if store == nil || mailer == nil || len([]byte(secret)) < 32 {
		return nil, errors.New("email verification requires a store, mailer, and 32-byte secret")
	}
	return &VerificationService{store: store, mailer: mailer, secret: []byte(secret), now: func() time.Time { return time.Now().UTC() }, generate: newCode, expiresIn: 10 * time.Minute}, nil
}

/**
 * Send 用于发送对应消息或请求。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param rawEmail 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * Verify 用于校验输入或运行状态是否满足要求。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param rawEmail 本次操作需要使用的输入参数。
 * @param code 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * hash 封装该名称对应的业务处理逻辑。
 * @param email 本次操作需要使用的输入参数。
 * @param code 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *VerificationService) hash(email, code string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(email + "\x00" + code))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

/**
 * newCode 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func newCode() (string, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(random[:])%1000000), nil
}
