package ratelimit

import (
	"net/http"

	"golang.org/x/time/rate"

	"cloud-native-reverse-proxy/pkg/middleware"
)

type RateLimit struct {
	limiter *rate.Limiter
	next    http.Handler
}

func (rl *RateLimit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !rl.limiter.Allow() {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	rl.next.ServeHTTP(w, r)
}

func New(limiter *rate.Limiter) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return &RateLimit{
			limiter: limiter,
			next:    next,
		}
	}
}
