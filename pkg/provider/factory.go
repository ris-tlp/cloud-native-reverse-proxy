package provider

import (
	"fmt"

	"cloud-native-reverse-proxy/internal/config"

	"github.com/moby/moby/client"
)

func Build(cfg config.ProvidersConfig) ([]Provider, error) {
	var providers []Provider

	if cfg.Docker.Enabled {
		cli, err := client.New(client.FromEnv)
		if err != nil {
			return nil, fmt.Errorf("docker: %w", err)
		}
		providers = append(providers, NewDockerProvider("docker", cli))
	}

	return providers, nil
}
