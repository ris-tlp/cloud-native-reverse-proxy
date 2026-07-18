package main

import (
	"log/slog"
	"os"

	"cloud-native-reverse-proxy/internal/config"
	"cloud-native-reverse-proxy/pkg/middleware"
)

func buildMiddlewares(cfg config.MiddlewareConfig) ([]middleware.Middleware, error) {
	var middlewares []middleware.Middleware

	if cfg.Logging.Enabled {
		var level slog.LevelVar
		level.Set(cfg.Logging.Level)

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: &level,
		}))

		middlewares = append(middlewares, middleware.NewLoggingMiddleware(logger))
	}

	return middlewares, nil
}
