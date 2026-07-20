// Package cors
package cors

import (
	"net/http"
	"slices"
	"strings"

	"cloud-native-reverse-proxy/pkg/middleware"
)

type CORS struct {
	allowedOrigins []string
	allowedMethods string
	allowedHeaders string
	next           http.Handler
}

func (c *CORS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")

	switch {
	case slices.Contains(c.allowedOrigins, "*"):
		w.Header().Set("Access-Control-Allow-Origin", "*")
	case origin != "" && slices.Contains(c.allowedOrigins, origin):
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", c.allowedMethods)
		w.Header().Set("Access-Control-Allow-Headers", c.allowedHeaders)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	c.next.ServeHTTP(w, r)
}

func New(allowedOrigins, allowedMethods, allowedHeaders []string) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return &CORS{
			allowedOrigins: allowedOrigins,
			allowedMethods: strings.Join(allowedMethods, ", "),
			allowedHeaders: strings.Join(allowedHeaders, ", "),
			next:           next,
		}
	}
}
