// Package watcher
package watcher

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"
)

const (
	watcherBufferSize = 100
	watchMaxElapsed   = 10 * time.Minute
)

type Watcher struct {
	reg       *registry.Registry
	logger    *slog.Logger
	providers []provider.Provider
}

func NewWatcher(reg *registry.Registry, logger *slog.Logger, providers ...provider.Provider) *Watcher {
	return &Watcher{reg: reg, logger: logger, providers: providers}
}

// Run launches every provider and the consumer
func (w *Watcher) Run(ctx context.Context) error {
	watcherBuffer := make(chan provider.Event, watcherBufferSize)

	var wg sync.WaitGroup
	for _, p := range w.providers {
		wg.Add(1)
		go func(p provider.Provider) {
			defer wg.Done()
			w.runProvider(ctx, p, watcherBuffer)
		}(p)
	}

	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		w.updateRegistry(ctx, watcherBuffer)
	}()

	wg.Wait()
	<-consumerDone
	return ctx.Err()
}

// runProvider wraps a provider's Watch in exponential backoff
func (w *Watcher) runProvider(ctx context.Context, p provider.Provider, watcherBuffer chan<- provider.Event) {
	logger := w.logger.With("source", p.Name())

	b := backoff.WithContext(
		backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(watchMaxElapsed)),
		ctx,
	)
	err := backoff.Retry(func() error {
		return p.Watch(ctx, watcherBuffer, logger)
	}, b)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("provider stopped", "err", err)
	}
}

// updateRegistry is the single mutator of the registry
func (w *Watcher) updateRegistry(ctx context.Context, watcherBuffer <-chan provider.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-watcherBuffer:
			switch e := ev.(type) {
			case provider.Change:
				w.updateRoute(ctx, e)
			case provider.BatchChange:
				w.reconcile(ctx, e)
			}
		}
	}
}

// reconcile diffs a source's full route set against the registry to correct drift
func (w *Watcher) reconcile(ctx context.Context, b provider.BatchChange) {
	desired := make(map[string]*registry.Route, len(b.Routes))
	for _, route := range b.Routes {
		desired[route.Host] = route
	}

	for _, route := range desired {
		w.updateRoute(ctx, provider.Change{Op: provider.OpRegister, Source: b.Source, Host: route.Host, Route: route})
	}

	// drop orphan hosts
	for _, host := range w.reg.HostsBySource(b.Source) {
		if _, ok := desired[host]; !ok {
			w.updateRoute(ctx, provider.Change{Op: provider.OpDeregister, Source: b.Source, Host: host})
		}
	}

	// drop orphan backends
	for host, desiredRoute := range desired {
		existing := w.reg.Lookup(host)
		if existing == nil {
			continue
		}
		desiredTargets := make(map[string]struct{}, len(desiredRoute.Backends))
		for _, b := range desiredRoute.Backends {
			desiredTargets[b.Target.String()] = struct{}{}
		}
		var stale []*registry.Backend
		for _, b := range existing.Backends {
			if _, ok := desiredTargets[b.Target.String()]; !ok {
				stale = append(stale, b)
			}
		}
		for _, b := range stale {
			w.reg.Deregister(host, b.Target)
			w.logger.Info("deregistered stale backend", "host", host, "target", b.Target.Host)
		}
	}
}

// updateRoute applies a single Change after a naive TCP healthcheck
func (w *Watcher) updateRoute(ctx context.Context, c provider.Change) {
	logger := w.logger.With("source", c.Source, "host", c.Host)
	switch c.Op {
	case provider.OpRegister:
		for _, b := range c.Route.Backends {
			if err := b.Proxy.Check(ctx); err != nil {
				logger.Warn("health check failed, skipping registration", "target", b.Target, "err", err)
				return
			}
		}
		w.reg.Register(c.Route)
		logger.Info("registered route", "backends", len(c.Route.Backends), "targets", c.Route.Backends)
	case provider.OpDeregister:
		var target *url.URL
		if c.Route != nil {
			target = c.Route.Backends[0].Target
		}
		w.reg.Deregister(c.Host, target)
		logger.Info("deregistered route", "target", c.Route)
	default:
		logger.Warn("unknown change op", "op", c.Op)
	}
}
