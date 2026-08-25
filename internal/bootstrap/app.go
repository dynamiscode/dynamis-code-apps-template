package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type App struct {
	DB *sql.DB
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return &App{DB: db}, nil
}

func Run(ctx context.Context, cfg config.Config) error {
	app, err := New(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.Close()

	<-ctx.Done()
	return nil
}

func (a *App) Close() error {
	return a.DB.Close()
}
