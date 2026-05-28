// Package registry
package registry

import (
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	routes map[string]*Route
}

func NewRegistry() *Registry {
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

func (r *Registry) Hosts() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	hosts := make([]string, 0, len(r.routes))
	for h := range r.routes {
		hosts = append(hosts, h)
	}
	return hosts
}

func (r *Registry) HostsBySource(source string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	hosts := make([]string, 0)
	for h, route := range r.routes {
		if route.Source == source {
			hosts = append(hosts, h)
		}
	}
	return hosts
}
