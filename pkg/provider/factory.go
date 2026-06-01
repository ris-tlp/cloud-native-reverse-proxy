package provider

import (
	"context"
	"fmt"
	"time"

	"cloud-native-reverse-proxy/internal/config"

	"github.com/moby/moby/client"
)

func Build(ctx context.Context, cfg config.ProvidersConfig) ([]Provider, error) {
	var providers []Provider

	if cfg.Docker.Enabled {
		opts := []client.Opt{client.FromEnv}
		if cfg.Docker.Host != "" {
			opts = append(opts, client.WithHost(cfg.Docker.Host))
		}
		cli, err := client.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("docker: %w", err)
		}
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := cli.Ping(pingCtx, client.PingOptions{}); err != nil {
			host := cfg.Docker.Host
			if host == "" {
				host = "default"
			}
			return nil, fmt.Errorf("docker: cannot reach host %q: %w", host, err)
		}
		providers = append(providers, NewDockerProvider("docker", cli))
	}

	return providers, nil
}
