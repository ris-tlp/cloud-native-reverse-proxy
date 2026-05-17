package registry

import (
	"net/http/httputil"
	"net/url"

	"cloud-native-reverse-proxy/pkg/proxy"
)

type Route struct {
	Host  string
	URL   string
	Proxy *httputil.ReverseProxy
}

func NewRoute(host string, target *url.URL) *Route {
	return &Route{
		Host:  host,
		URL:   target.String(),
		Proxy: proxy.New(target),
	}
}
