package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server    ServerConfig    `toml:"server"`
	Providers ProvidersConfig `toml:"providers"`
}

type ServerConfig struct {
	Port int `toml:"port"`
}

type ProvidersConfig struct {
	Docker DockerConfig `toml:"docker"`
}

type DockerConfig struct {
	Enabled bool   `toml:"enabled"`
	Host    string `toml:"host"`
}

func defaults() Config {
	return Config{
		Server: ServerConfig{Port: 8080},
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
