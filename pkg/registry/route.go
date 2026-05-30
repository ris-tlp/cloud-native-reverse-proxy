package registry

import (
	"log/slog"
	"net/url"

	"github.com/samber/lo"

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
	return slog.GroupValue(
		slog.String("host", r.Host),
		slog.String("source", r.Source),
		slog.Int("backends", len(r.Backends)),
		slog.Any("targets", lo.Map(r.Backends, func(b *Backend, _ int) string { return b.Target.Host })),
	)
}

func (route *Route) AddBackend(backend *Backend) {
	if lo.ContainsBy(route.Backends, func(b *Backend) bool {
		return b.Target.String() == backend.Target.String()
	}) {
		return
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
