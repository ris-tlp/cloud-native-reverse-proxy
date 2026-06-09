package proxy

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

var _ Proxy = (*SimpleProxy)(nil)

type SimpleProxy struct {
	target *url.URL
	rp     *httputil.ReverseProxy
}

func NewSimple(target *url.URL) *SimpleProxy {
	return &SimpleProxy{
		target: target,
		rp: &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(target)
				pr.SetXForwarded()
				pr.Out.Host = target.Host
			},
			Transport:    defaultTransport(),
			ErrorHandler: defaultErrorHandler,
		},
	}
}

func (p *SimpleProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.rp.ServeHTTP(w, r)
}

func defaultTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func defaultErrorHandler(w http.ResponseWriter, req *http.Request, err error) {
	slog.Error("proxy error", "host", req.Host, "err", err)
	http.Error(w, "backend unavailable", http.StatusBadGateway)
}
