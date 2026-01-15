package main

import (
	"api_gateway/internal/proxy"
	"api_gateway/internal/server"
	"net/http"
	"net/http/httputil"
)

func proxyHandler(p *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.ServeHTTP(w, r)
	}
}

func main() {
	s := server.GetServer()
	http.HandleFunc("/", proxyHandler(proxy.GetProxy("http://localhost:9000")))
	err := s.ListenAndServe()
	if err != nil {
		return
	}
}
