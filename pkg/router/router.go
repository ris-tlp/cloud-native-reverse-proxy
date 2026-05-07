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

func (router *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	router.mux.ServeHTTP(w, req)
}
