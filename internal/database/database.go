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

func quoteIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}
