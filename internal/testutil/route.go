package testutil

import (
	"cloud-native-reverse-proxy/pkg/registry"
)

func TestRoute(host, source string, targets ...string) *registry.Route {
	backends := make([]*registry.Backend, len(targets))
	for i, target := range targets {
		backends[i] = &registry.Backend{
			Target: MustURL("http://" + target),
			Proxy:  &MockProxy{Healthy: true},
		}
	}
	return &registry.Route{
		Host:         host,
		Source:       source,
		Backends:     backends,
		LoadBalancer: registry.NewLoadBalancer(""),
	}
}
