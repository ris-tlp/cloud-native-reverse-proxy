package proxy

import (
	"net/http/httputil"
	"net/url"
)

func GetProxy(target string) *httputil.ReverseProxy {
	upstream, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	return proxy
}
