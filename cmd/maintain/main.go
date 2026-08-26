package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/maintenance"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		fail(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		fail(err)
	}
	result, err := maintenance.Run(ctx, db, cfg.Database.Driver, time.Now(), cfg.Data.AuditRetention)
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
