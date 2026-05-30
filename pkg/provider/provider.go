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

// Event wraps Change and BatchChange events
type Event interface {
	isEvent()
}

// Change is a single incremental update emitted per container event
type Change struct {
	Op     ChangeOp
	Source string
	Host   string
	Route  *registry.Route
}

func (Change) isEvent() {}

// BatchChange is the full route set for a source for reconciliation
type BatchChange struct {
	Source string
	Routes []*registry.Route
}

func (BatchChange) isEvent() {}

// Provider is solely used to source routes and announce changes
type Provider interface {
	Name() string
	Watch(ctx context.Context, events chan<- Event, logger *slog.Logger) error
}
