// Package registry
package registry

import (
	"net/url"
	"slices"
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
	existingRoute, ok := r.routes[route.Host]
	if ok {
		for _, b := range route.Backends {
			existingRoute.AddBackend(b)
		}
	} else {
		r.routes[route.Host] = route
	}
}

func (r *Registry) Deregister(host string, target *url.URL) {
	r.mu.Lock()
	defer r.mu.Unlock()
	route, ok := r.routes[host]
	if !ok {
		return
	}
	if target == nil || len(route.Backends) <= 1 {
		delete(r.routes, host)
		return
	}
	// remove just this backend
	route.Backends = slices.DeleteFunc(route.Backends, func(b *Backend) bool {
		return b.Target.String() == target.String()
	})
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
