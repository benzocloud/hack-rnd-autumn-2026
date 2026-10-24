package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/config"
)

func main() { os.Exit(run()) }

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) != 1 {
		logger.Error("usage: migrate (applies pending up migrations only)")
		return 1
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("auth configuration invalid", "error", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := app.Migrate(ctx, cfg); err != nil {
		logger.Error("auth migration stopped", "error", err)
		return 1
	}
	logger.Info("auth migrations applied")
	return 0
}
