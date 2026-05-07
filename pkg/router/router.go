// Package router
package router

import (
	"io"
	"net/http"
)

type Router struct {
	mux *http.ServeMux
}

func New() *Router {
	router := &Router{
		mux: http.NewServeMux(),
	}

	router.mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, "Health check")
	})

	return router
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}
