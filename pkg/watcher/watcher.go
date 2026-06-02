// Package watcher
package watcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/samber/lo"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"
)

const (
	watcherBufferSize = 100
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

func (w *Watcher) runProvider(ctx context.Context, p provider.Provider, watcherBuffer chan<- provider.Event) {
	logger := w.logger.With("source", p.Name())
	b := backoff.WithContext(backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(0)), ctx)
	notify := func(err error, d time.Duration) {
		logger.Warn("provider disconnected, retrying", "err", err, "in", d)
	}
	err := backoff.RetryNotify(func() error { return recoverableWatch(ctx, p, watcherBuffer, logger) }, b, notify)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("provider stopped", "err", err)
	}
}

func recoverableWatch(ctx context.Context, p provider.Provider, watcherBuffer chan<- provider.Event, logger *slog.Logger) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()
	return p.Watch(ctx, watcherBuffer, logger)
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
	desired := lo.KeyBy(b.Routes, func(r *registry.Route) string { return r.Host })

	for _, route := range desired {
		w.updateRoute(ctx, provider.Change{Op: provider.OpRegister, Host: route.Host, Route: route})
	}

	// drop orphan hosts
	for _, host := range w.reg.HostsBySource(b.Source) {
		if _, ok := desired[host]; !ok {
			w.updateRoute(ctx, provider.Change{Op: provider.OpDeregister, Host: host})
		}
	}

	// drop orphan backends
	for host, desiredRoute := range desired {
		existing := w.reg.Lookup(host)
		if existing == nil {
			continue
		}
		desiredTargets := lo.SliceToMap(desiredRoute.Backends, func(be *registry.Backend) (string, struct{}) {
			return be.Target.String(), struct{}{}
		})
		stale := lo.Filter(existing.Backends, func(be *registry.Backend, _ int) bool {
			_, ok := desiredTargets[be.Target.String()]
			return !ok
		})
		for _, be := range stale {
			w.reg.Deregister(host, be.Target)
			w.logger.Info("deregistered stale backend", "host", host, "target", be.Target.Host)
		}
	}
}

// updateRoute applies a single Change after a naive TCP healthcheck
func (w *Watcher) updateRoute(ctx context.Context, c provider.Change) {
	logger := w.logger.With("host", c.Host)
	switch c.Op {
	case provider.OpRegister:
		for _, b := range c.Route.Backends {
			if err := b.Proxy.Check(ctx); err != nil {
				logger.Warn("health check failed, skipping registration", "target", b.Target, "err", err)
				return
			}
		}
		w.reg.Register(c.Route)
		logger.Info("registered route", "route", c.Route)
	case provider.OpDeregister:
		var target *url.URL
		if c.Route != nil {
			target = c.Route.Backends[0].Target
		}
		w.reg.Deregister(c.Host, target)
		logger.Info("deregistered route", "route", c.Route)
	default:
		logger.Warn("unknown change op", "op", c.Op)
	}
}
