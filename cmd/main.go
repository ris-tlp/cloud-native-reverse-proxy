package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"
	"cloud-native-reverse-proxy/pkg/router"
	"cloud-native-reverse-proxy/pkg/server"
	"cloud-native-reverse-proxy/pkg/watcher"

	"github.com/moby/moby/client"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	logger := slog.Default()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := registry.NewRegistry()
	r := router.New(reg)
	srv := server.New(":8080", r)

	dockerCli, err := client.New(client.FromEnv)
	if err != nil {
		slog.Error("failed to create docker client", "err", err)
		os.Exit(1)
	}
	dockerProvider := provider.NewDockerProvider("docker", dockerCli)

	w := watcher.NewWatcher(reg, logger, dockerProvider)
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
