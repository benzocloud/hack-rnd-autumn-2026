package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/app/server"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	coreLog "github.com/benzocloud/hack-rnd-autumn-2026/services/core/pkg/log"
)

func main() { os.Exit(run()) }

func run() int {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("Core configuration invalid\n")
		return 1
	}
	logger, err := coreLog.New(os.Stdout, cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		return 1
	}
	rpc, err := config.LoadRPC()
	if err != nil {
		logger.Error("Core RPC configuration invalid")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, cfg, rpc); err != nil {
		logger.Error("Core stopped with a runtime error")
		return 1
	}
	return 0
}
