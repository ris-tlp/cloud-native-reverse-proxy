package registry

import (
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
