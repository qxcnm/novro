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

func (a *App) Close() error {
	if a == nil || a.Ent == nil {
		return nil
	}
	return a.Ent.Close()
}
