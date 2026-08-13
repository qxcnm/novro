package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/novro-gateway/novro/internal/config"
)

// EnsureDatabase creates the configured database before application
// migrations run. It is intended for one-time use with an administrator
// account; normal application startup never calls it.
/**
 * EnsureDatabase 用于校验输入或运行状态是否满足要求。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param cfg 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func EnsureDatabase(ctx context.Context, cfg config.DatabaseConfig) error {
	if cfg.Driver != "mysql" {
		return fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
	mysqlConfig := cfg.MySQLConfig()
	mysqlConfig.DBName = ""
	connector, err := mysql.NewConnector(mysqlConfig)
	if err != nil {
		return fmt.Errorf("create database bootstrap connector: %w", err)
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()
	statement := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci",
		quoteIdentifier(cfg.Name),
	)
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	return nil
}

/**
 * Open 用于解密并返回受保护的数据。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param cfg 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func Open(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	if cfg.Driver != "mysql" {
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
	mysqlConfig := cfg.MySQLConfig()
	connector, err := mysql.NewConnector(mysqlConfig)
	if err != nil {
		return nil, fmt.Errorf("create database connector: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

/**
 * quoteIdentifier 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func quoteIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}
