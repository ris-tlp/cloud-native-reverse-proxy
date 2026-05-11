// Package registry
package registry

import (
	"net/http/httputil"
	"sync"
)

type Route struct {
	Host  string
	URL   string
	Proxy *httputil.ReverseProxy
}

type Registry struct {
	mu     sync.RWMutex
	routes map[string]*Route
}

func New() *Registry {
	return &Registry{routes: map[string]*Route{}}
}

func (r *Registry) Register(route *Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[route.Host] = route
}

func (r *Registry) Deregister(host string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.routes, host)
}

func (r *Registry) Lookup(host string) *Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.routes[host]
}
