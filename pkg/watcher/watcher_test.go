package watcher

import (
	"context"
	"testing"
	"time"

	"cloud-native-reverse-proxy/internal/testutil"
	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newWatcher(reg *registry.Registry) *Watcher {
	return NewWatcher(reg, testutil.DiscardLogger())
}

func TestReconcile_NewRouteRegistered(t *testing.T) {
	reg := registry.NewRegistry()
	w := newWatcher(reg)

	w.reconcile(context.Background(), provider.BatchChange{
		Source: "docker",
		Routes: []*registry.Route{testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000")},
	})

	got := reg.Lookup("app.localhost")
	require.NotNil(t, got)
	assert.Equal(t, "app.localhost", got.Host)
	assert.Len(t, got.Backends, 1)
}

func TestReconcile_OrphanHostDeregistered(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"))
	w := newWatcher(reg)

	// BatchChange with no routes — "app.localhost" should be removed
	w.reconcile(context.Background(), provider.BatchChange{
		Source: "docker",
		Routes: []*registry.Route{},
	})

	assert.Nil(t, reg.Lookup("app.localhost"))
}

func TestReconcile_OrphanBackendDeregistered(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000", "10.0.0.2:3000"))
	w := newWatcher(reg)

	// Desired has only one backend — the second should be pruned
	w.reconcile(context.Background(), provider.BatchChange{
		Source: "docker",
		Routes: []*registry.Route{testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000")},
	})

	got := reg.Lookup("app.localhost")
	require.NotNil(t, got)
	require.Len(t, got.Backends, 1)
	assert.Equal(t, "10.0.0.1:3000", got.Backends[0].Target.Host)
}

func TestReconcile_NoChangeOnMatch(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"))
	w := newWatcher(reg)

	w.reconcile(context.Background(), provider.BatchChange{
		Source: "docker",
		Routes: []*registry.Route{testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000")},
	})

	got := reg.Lookup("app.localhost")
	require.NotNil(t, got)
	assert.Len(t, got.Backends, 1)
}

func TestReconcile_CrossSourcePreserved(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"))
	reg.Register(testutil.TestRoute("svc.localhost", "kubernetes", "10.0.0.2:3000"))
	w := newWatcher(reg)

	// Docker BatchChange with no routes — should only remove docker-owned routes
	w.reconcile(context.Background(), provider.BatchChange{
		Source: "docker",
		Routes: []*registry.Route{},
	})

	assert.Nil(t, reg.Lookup("app.localhost"), "docker route should be removed")
	assert.NotNil(t, reg.Lookup("svc.localhost"), "kubernetes route should be preserved")
}

func TestReconcile_UnhealthyBackendSkipped(t *testing.T) {
	reg := registry.NewRegistry()
	w := newWatcher(reg)

	unhealthyRoute := &registry.Route{
		Host:   "app.localhost",
		Source: "docker",
		Backends: []*registry.Backend{
			{Target: testutil.MustURL("http://10.0.0.1:3000"), Proxy: &testutil.MockProxy{Healthy: false}},
		},
		LoadBalancer: registry.NewLoadBalancer(""),
	}

	w.reconcile(context.Background(), provider.BatchChange{
		Source: "docker",
		Routes: []*registry.Route{unhealthyRoute},
	})

	assert.Nil(t, reg.Lookup("app.localhost"), "unhealthy backend should not be registered")
}

func TestUpdateRoute_RegisterHealthy(t *testing.T) {
	reg := registry.NewRegistry()
	w := newWatcher(reg)

	w.updateRoute(context.Background(), provider.Change{
		Op:    provider.OpRegister,
		Host:  "app.localhost",
		Route: testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"),
	})

	got := reg.Lookup("app.localhost")
	require.NotNil(t, got)
	assert.Equal(t, "app.localhost", got.Host)
}

func TestUpdateRoute_RegisterUnhealthy(t *testing.T) {
	reg := registry.NewRegistry()
	w := newWatcher(reg)

	route := &registry.Route{
		Host:         "app.localhost",
		Source:       "docker",
		Backends:     []*registry.Backend{{Target: testutil.MustURL("http://10.0.0.1:3000"), Proxy: &testutil.MockProxy{Healthy: false}}},
		LoadBalancer: registry.NewLoadBalancer(""),
	}

	w.updateRoute(context.Background(), provider.Change{
		Op:    provider.OpRegister,
		Host:  "app.localhost",
		Route: route,
	})

	assert.Nil(t, reg.Lookup("app.localhost"))
}

func TestUpdateRoute_DeregisterWithRoute(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"))
	w := newWatcher(reg)

	w.updateRoute(context.Background(), provider.Change{
		Op:    provider.OpDeregister,
		Host:  "app.localhost",
		Route: testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"),
	})

	assert.Nil(t, reg.Lookup("app.localhost"))
}

func TestUpdateRoute_DeregisterNilRoute(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"))
	w := newWatcher(reg)

	w.updateRoute(context.Background(), provider.Change{
		Op:   provider.OpDeregister,
		Host: "app.localhost",
	})

	assert.Nil(t, reg.Lookup("app.localhost"))
}

func TestUpdateRoute_UnknownOpNoChange(t *testing.T) {
	reg := registry.NewRegistry()
	reg.Register(testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"))
	w := newWatcher(reg)

	w.updateRoute(context.Background(), provider.Change{
		Op:   provider.ChangeOp("unknown"),
		Host: "app.localhost",
	})

	assert.NotNil(t, reg.Lookup("app.localhost"))
}

func TestUpdateRegistry_DispatchesChange(t *testing.T) {
	reg := registry.NewRegistry()
	w := newWatcher(reg)
	ch := make(chan provider.Event, 1)
	ctx := t.Context()

	ch <- provider.Change{
		Op:    provider.OpRegister,
		Host:  "app.localhost",
		Route: testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000"),
	}

	go w.updateRegistry(ctx, ch)

	require.Eventually(t, func() bool {
		return reg.Lookup("app.localhost") != nil
	}, time.Second, 10*time.Millisecond)
}

func TestUpdateRegistry_DispatchesBatchChange(t *testing.T) {
	reg := registry.NewRegistry()
	w := newWatcher(reg)
	ch := make(chan provider.Event, 1)
	ctx := t.Context()

	ch <- provider.BatchChange{
		Source: "docker",
		Routes: []*registry.Route{testutil.TestRoute("app.localhost", "docker", "10.0.0.1:3000")},
	}

	go w.updateRegistry(ctx, ch)

	require.Eventually(t, func() bool {
		return reg.Lookup("app.localhost") != nil
	}, time.Second, 10*time.Millisecond)
}

func TestUpdateRegistry_ExitsOnContextCancel(t *testing.T) {
	reg := registry.NewRegistry()
	w := newWatcher(reg)
	ch := make(chan provider.Event)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.updateRegistry(ctx, ch)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("updateRegistry did not exit after context cancel")
	}
}
