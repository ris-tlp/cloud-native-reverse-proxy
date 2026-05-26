package registry

import (
	"net/http/httputil"
	"net/url"

	"cloud-native-reverse-proxy/pkg/proxy"
)

type Route struct {
	Host   string
	Target *url.URL
	Proxy  *httputil.ReverseProxy
}

func NewRoute(host string, target *url.URL) *Route {
	return &Route{
		Host:   host,
		Target: target,
		Proxy:  proxy.New(target),
	}
}
