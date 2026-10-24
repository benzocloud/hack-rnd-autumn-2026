package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/app"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/config"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cfg, err := config.Load()
	if err == nil {
		err = app.Run(ctx, cfg, logger)
	}
	if err != nil {
		logger.Error("API stopped", "reason", "startup or lifecycle failure")
		return 1
	}
	return 0
}
