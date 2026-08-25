package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"example.com/dynamis-code/apps-template/internal/bootstrap"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration invalid", "error", err)
		os.Exit(1)
	}

	if err := bootstrap.Run(ctx, cfg); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
