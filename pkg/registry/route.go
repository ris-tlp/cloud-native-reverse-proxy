package registry

import (
	"log/slog"
	"net/url"

	"cloud-native-reverse-proxy/pkg/proxy"
)

type Route struct {
	Host         string
	Source       string
	Backends     []*Backend
	LoadBalancer LoadBalancer
}

func NewRoute(host string, target *url.URL, source string, loadBalancer LoadBalancer) *Route {
	return &Route{
		Host:         host,
		Source:       source,
		Backends:     []*Backend{NewBackend(target)},
		LoadBalancer: loadBalancer,
	}
}

func (r *Route) LogValue() slog.Value {
	targets := make([]string, len(r.Backends))
	for i, b := range r.Backends {
		targets[i] = b.Target.Host
	}
	return slog.GroupValue(
		slog.String("host", r.Host),
		slog.String("source", r.Source),
		slog.Int("backends", len(r.Backends)),
		slog.Any("targets", targets),
	)
}

func (route *Route) AddBackend(backend *Backend) {
	for _, b := range route.Backends {
		if b.Target.String() == backend.Target.String() {
			return
		}
	}
	route.Backends = append(route.Backends, backend)
}

type Backend struct {
	Target *url.URL
	Proxy  proxy.Proxy
}

func NewBackend(target *url.URL) *Backend {
	return &Backend{
		Target: target,
		Proxy:  proxy.NewSimple(target),
	}
}

func (b *Backend) LogValue() slog.Value {
	return slog.StringValue(b.Target.Host)
}
