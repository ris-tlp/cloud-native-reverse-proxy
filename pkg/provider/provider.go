// Package provider
package provider

import (
	"context"
	"log/slog"

	"cloud-native-reverse-proxy/pkg/registry"
)

type ChangeOp string

const (
	OpRegister   ChangeOp = "register"
	OpDeregister ChangeOp = "deregister"
)

// Change is a framework level event that the watcher will consume to update the registry
type Change struct {
	Op     ChangeOp
	Source string          // name of the emitting provider
	Host   string          // route key (matches Route.Host for OpRegister)
	Route  *registry.Route // populated for OpRegister; nil for OpDeregister
}

// Provider is solely used to source routes and announce changes
type Provider interface {
	Name() string
	Watch(ctx context.Context, changes chan<- Change, logger *slog.Logger) error
}
