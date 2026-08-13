package email

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

type SQLStore struct{ db *sql.DB }

/**
 * NewSQLStore 用于创建并返回所需的对象或记录。
 * @param db 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

/**
 * Issue 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param email 本次操作需要使用的输入参数。
 * @param hash 控制对应行为是否启用的布尔值。
 * @param expiresAt 本次操作需要使用的输入参数。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *SQLStore) Issue(ctx context.Context, email, hash string, expiresAt, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin verification code transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT created_at FROM email_verification_codes WHERE email = ? FOR UPDATE`, email).Scan(&createdAt)
	switch {
	case err == nil:
		if createdAt.After(now.Add(-time.Minute)) {
			return ErrRateLimited
		}
		if _, err := tx.ExecContext(ctx, `UPDATE email_verification_codes SET code_hash = ?, expires_at = ?, consumed_at = NULL, created_at = ? WHERE email = ?`, hash, expiresAt, now, email); err != nil {
			return fmt.Errorf("replace verification code: %w", err)
		}
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO email_verification_codes (id, email, code_hash, expires_at, consumed_at, created_at) VALUES (?, ?, ?, ?, NULL, ?)`, uuid.New().String(), email, hash, expiresAt, now); err != nil {
			var mysqlErr *mysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				return ErrRateLimited
			}
			return fmt.Errorf("save verification code: %w", err)
		}
	default:
		return fmt.Errorf("lock verification code: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit verification code: %w", err)
	}
	return nil
}

/**
 * Consume 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param email 本次操作需要使用的输入参数。
 * @param hash 控制对应行为是否启用的布尔值。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *SQLStore) Consume(ctx context.Context, email, hash string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin verification check transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var id, storedHash string
	var expiresAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT id, code_hash, expires_at FROM email_verification_codes WHERE email = ? AND consumed_at IS NULL ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, email).Scan(&id, &storedHash, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCode
		}
		return fmt.Errorf("read verification code: %w", err)
	}
	if !expiresAt.After(now) {
		return ErrExpired
	}
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(hash)) != 1 {
		return ErrInvalidCode
	}
	if _, err := tx.ExecContext(ctx, `UPDATE email_verification_codes SET consumed_at = ? WHERE id = ?`, now, id); err != nil {
		return fmt.Errorf("consume verification code: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit verification check: %w", err)
	}
	return nil
}

/**
 * DeleteIssue 用于删除、撤销或释放指定资源。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param email 本次操作需要使用的输入参数。
 * @param hash 控制对应行为是否启用的布尔值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *SQLStore) DeleteIssue(ctx context.Context, email, hash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM email_verification_codes WHERE email = ? AND code_hash = ?`, email, hash)
	return err
}
