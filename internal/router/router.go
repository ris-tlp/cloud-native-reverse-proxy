package router

import (
	"net/http"
	"net/http/httputil"
	"strings"
)

type Route struct {
	Host       string
	PathPrefix string
	Proxy      *httputil.ReverseProxy
}

type DockerLabels struct {
	Host       string
	PathPrefix string
	//UpstreamURL *url.URL
}

type Router struct {
	routes []Route
}

func NewRouter(routes []Route) *Router {
	return &Router{routes: routes}
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, route := range rt.routes {
		if r.Host == route.Host &&
			strings.HasPrefix(r.URL.Path, route.PathPrefix) {

			route.Proxy.ServeHTTP(w, r)
			return
		}
	}

	http.NotFound(w, r)
}
