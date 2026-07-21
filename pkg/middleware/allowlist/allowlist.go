// Package allowlist
package allowlist

import (
	"net"
	"net/http"

	"cloud-native-reverse-proxy/pkg/middleware"
)

type Allowlist struct {
	allowed map[string]bool
	next    http.Handler
}

func (a *Allowlist) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	if !a.allowed[host] {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	a.next.ServeHTTP(w, r)
}

func New(allowedIPs []string) middleware.Middleware {
	allowed := make(map[string]bool, len(allowedIPs))
	for _, ip := range allowedIPs {
		allowed[ip] = true
	}

	return func(next http.Handler) http.Handler {
		return &Allowlist{
			allowed: allowed,
			next:    next,
		}
	}
}
