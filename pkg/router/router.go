// Package router
package router

import (
	"io"
	"net/http"

	"cloud-native-reverse-proxy/pkg/middleware"
	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/justinas/alice"
)

type Router struct {
	mux *http.ServeMux
}

func New(reg *registry.Registry, middlewares []middleware.Middleware) *Router {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	constructors := make([]alice.Constructor, len(middlewares))
	for i, mw := range middlewares {
		constructors[i] = alice.Constructor(mw)
	}

	middlewareChain := alice.New(constructors...).Then(proxyHandler(reg))

	mux.Handle("/", middlewareChain)
	return &Router{mux: mux}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req) // no branches, mux dispatches
}

func proxyHandler(reg *registry.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		route := reg.Lookup(req.Host)
		if route == nil {
			http.NotFound(w, req)
			return
		}
		backend := route.LoadBalancer.Pick(route.Backends)
		if backend == nil {
			http.NotFound(w, req)
			return
		}
		backend.Proxy.ServeHTTP(w, req)
	})
}

func healthHandler(w http.ResponseWriter, req *http.Request) {
	_, _ = io.WriteString(w, "Healthy")
}
