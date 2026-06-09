// Package proxy
package proxy

import (
	"net/http"
)

type Proxy interface {
	http.Handler
}
