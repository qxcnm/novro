package app

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/novro-gateway/novro/ent"
	"github.com/novro-gateway/novro/internal/config"
	"github.com/novro-gateway/novro/internal/database"
)

type App struct {
	Config config.Config
	DB     *sql.DB
	Ent    *ent.Client
}

/**
 * New 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param cfg 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func New(ctx context.Context, cfg config.Config) (*App, error) {
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	driver := entsql.OpenDB(dialect.MySQL, db)
	return &App{
		Config: cfg,
		DB:     db,
		Ent:    ent.NewClient(ent.Driver(driver)),
	}, nil
}

/**
 * Close 用于删除、撤销或释放指定资源。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (a *App) Close() error {
	if a == nil || a.Ent == nil {
		return nil
	}
	return a.Ent.Close()
}
