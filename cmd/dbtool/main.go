package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/backup"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "backup" && os.Args[1] != "restore") {
		fail("usage: dbtool backup|restore -file PATH")
	}
	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ExitOnError)
	file := flags.String("file", "", "backup file")
	target := flags.String("target", "", "new SQLite restore path")
	maxAge := flags.Duration("max-age", 0, "reject backup evidence older than this duration")
	if err := flags.Parse(os.Args[2:]); err != nil {
		fail(err.Error())
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if command == "restore" {
		if err := backup.Restore(ctx, cfg.Database, *file, *target, time.Now(), *maxAge); err != nil {
			fail(err.Error())
		}
		fmt.Println("restore verified")
		return
	}
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		fail(err.Error())
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		fail(err.Error())
	}
	manifest, err := backup.Create(ctx, db, cfg.Database, *file, time.Now())
	if err != nil {
		fail(err.Error())
	}
	fmt.Printf("backup verified: driver=%s created_at=%s\n", manifest.Driver, manifest.CreatedAt.Format(time.RFC3339))
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
