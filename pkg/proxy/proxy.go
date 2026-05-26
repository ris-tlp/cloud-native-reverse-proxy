// Package proxy
package proxy

import (
	"context"
	"net/http"
)

type Proxy interface {
	http.Handler
	Check(ctx context.Context) error
}
