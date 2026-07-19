package config

import (
	"errors"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server    ServerConfig    `toml:"server"`
	Providers ProvidersConfig `toml:"providers"`
}

type ServerConfig struct {
	Port       int              `toml:"port"`
	LogLevel   slog.Level       `toml:"logLevel"`
	Middleware MiddlewareConfig `toml:"middleware"`
}

type MiddlewareConfig struct {
	Logging   LoggingConfig   `toml:"logging"`
	RateLimit RateLimitConfig `toml:"ratelimit"`
}

type LoggingConfig struct {
	Enabled bool       `toml:"enabled"`
	Level   slog.Level `toml:"level"`
}

type RateLimitConfig struct {
	Enabled           bool    `toml:"enabled"`
	RequestsPerSecond float64 `toml:"requestsPerSecond"`
	Burst             int     `toml:"burst"`
}

type ProvidersConfig struct {
	Docker     DockerConfig     `toml:"docker"`
	Kubernetes KubernetesConfig `toml:"kubernetes"`
}

type DockerConfig struct {
	Enabled bool   `toml:"enabled"`
	Host    string `toml:"host"`
}

type KubernetesConfig struct {
	Ingress IngressConfig `toml:"ingress"`
}

type IngressConfig struct {
	Enabled      bool   `toml:"enabled"`
	IngressClass string `toml:"ingressClass"`
	Namespace    string `toml:"namespace"`
}

func defaults() Config {
	return Config{
		Server: ServerConfig{Port: 8080, LogLevel: slog.LevelInfo},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &cfg, nil
		}
		return nil, err
	}
	return &cfg, nil
}
