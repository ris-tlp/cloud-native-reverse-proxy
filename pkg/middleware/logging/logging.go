package logging

import (
	"log/slog"
	"net/http"
	"time"

	"cloud-native-reverse-proxy/pkg/middleware"
)

type Logging struct {
	logger *slog.Logger
	next   http.Handler
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (l *Logging) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

	l.next.ServeHTTP(rec, r)

	l.logger.Info(
		"http request",
		"method", r.Method,
		"host", r.Host,
		"path", r.URL.Path,
		"headers", r.Header,
		"status", rec.status,
		"duration", time.Since(start),
	)
}

func New(logger *slog.Logger) middleware.Middleware {
	return func(next http.Handler) http.Handler {
		return &Logging{
			next:   next,
			logger: logger,
		}
	}
}
