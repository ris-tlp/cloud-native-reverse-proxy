package registry

import (
	"net/http/httputil"
	"net/url"

	"cloud-native-reverse-proxy/pkg/proxy"
)

type Route struct {
	Host   string
	Target *url.URL
	Source string
	Proxy  *httputil.ReverseProxy
}

func NewRoute(host string, target *url.URL, source string) *Route {
	return &Route{
		Host:   host,
		Target: target,
		Source: source,
		Proxy:  proxy.New(target),
	}
}
