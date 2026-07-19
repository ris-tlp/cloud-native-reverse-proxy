package main

import (
	"log/slog"
	"os"

	"cloud-native-reverse-proxy/internal/config"
	"cloud-native-reverse-proxy/pkg/middleware"
	"cloud-native-reverse-proxy/pkg/middleware/logging"
	"cloud-native-reverse-proxy/pkg/middleware/ratelimit"

	"golang.org/x/time/rate"
)

func buildMiddlewares(cfg config.MiddlewareConfig) ([]middleware.Middleware, error) {
	var middlewares []middleware.Middleware

	if cfg.Logging.Enabled {
		var level slog.LevelVar
		level.Set(cfg.Logging.Level)

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: &level,
		}))

		middlewares = append(middlewares, logging.New(logger))
	}

	if cfg.RateLimit.Enabled {
		limiter := rate.NewLimiter(rate.Limit(cfg.RateLimit.RequestsPerSecond), cfg.RateLimit.Burst)
		middlewares = append(middlewares, ratelimit.New(limiter))
	}

	return middlewares, nil
}
