package main

import (
	"context"
	"fmt"
	"time"

	"cloud-native-reverse-proxy/internal/config"
	"cloud-native-reverse-proxy/pkg/provider"
	dockerProvider "cloud-native-reverse-proxy/pkg/provider/docker"
	kubernetesProvider "cloud-native-reverse-proxy/pkg/provider/kubernetes"

	"github.com/moby/moby/client"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const resyncPeriod = 10 * time.Minute

func buildProviders(ctx context.Context, cfg config.ProvidersConfig) ([]provider.Provider, error) {
	var providers []provider.Provider

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
		providers = append(providers, dockerProvider.New("docker", cli))
	}

	if cfg.Kubernetes.Ingress.Enabled {
		clusterCfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("kubernetes cluster configuration failed: %w", err)
		}
		clientset, err := kubernetes.NewForConfig(clusterCfg)
		if err != nil {
			return nil, fmt.Errorf("kubernetes clientset configuration failed: %w", err)
		}
		factory := informers.NewSharedInformerFactoryWithOptions(
			clientset,
			resyncPeriod,
			informers.WithNamespace(cfg.Kubernetes.Ingress.Namespace),
		)
		ingresses := factory.Networking().V1().Ingresses()
		providers = append(providers, kubernetesProvider.New("kubernetes", ingresses.Informer(), ingresses.Lister(), cfg.Kubernetes.Ingress.IngressClass))
	}

	return providers, nil
}
