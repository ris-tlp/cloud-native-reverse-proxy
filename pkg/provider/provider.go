// Package provider
package provider

import (
	"context"

	"cloud-native-reverse-proxy/pkg/registry"
)

// Provider sources routes and keeps the registry in sync with that source
type Provider interface {
	// Name returns a stable identifier for this provider instance,
	// used by the registry to scope ownership of routes
	Name() string

	// Subscribes to source events and runs periodic reconciliation internally on a ticker
	Watch(ctx context.Context, reg *registry.Registry) error

	// Reconcile performs a one-shot sync of the registry against the source's
	// current state for configuration drift
	Reconcile(ctx context.Context, reg *registry.Registry) error
}
