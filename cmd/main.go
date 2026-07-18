package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud-native-reverse-proxy/internal/config"
	"cloud-native-reverse-proxy/pkg/registry"
	"cloud-native-reverse-proxy/pkg/router"
	"cloud-native-reverse-proxy/pkg/server"
	"cloud-native-reverse-proxy/pkg/watcher"
)

func main() {
	configPath := flag.String("config", "cnrp.toml", "path to config file")
	flag.Parse()

	var level slog.LevelVar
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: &level,
	})))
	logger := slog.Default()

	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		slog.Warn("no config file found, using defaults", "path", *configPath)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("config file is malformed, fix or delete it to use defaults", "path", *configPath, "err", err)
		os.Exit(1)
	}
	level.Set(cfg.Server.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	providers, err := buildProviders(ctx, cfg.Providers)
	if err != nil {
		slog.Error("failed to build providers", "err", err)
		os.Exit(1)
	}
	if len(providers) == 0 {
		slog.Warn("no providers enabled")
	}

	middlewares, err := buildMiddlewares(cfg.Server.Middleware)
	if err != nil {
		slog.Info("failed to build middlewares", "err", err)
	}
	if len(middlewares) == 0 {
		slog.Info("no middlewares enabled")
	}

	reg := registry.NewRegistry()
	r := router.New(reg, middlewares)
	srv := server.New(fmt.Sprintf(":%d", cfg.Server.Port), r)

	w := watcher.NewWatcher(reg, logger, providers...)
	go func() {
		if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("watcher error", "err", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			slog.Error("server error", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "err", err)
	}
}
