package registry

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func TestRegister(t *testing.T) {
	t.Run("new host", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))
		got := reg.Lookup("app.localhost")
		require.NotNil(t, got)
		assert.Equal(t, "app.localhost", got.Host)
		assert.Len(t, got.Backends, 1)
	})

	t.Run("upsert merges new backend", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))
		reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.2:3000"), "docker", NewLoadBalancer("")))
		got := reg.Lookup("app.localhost")
		require.NotNil(t, got)
		assert.Len(t, got.Backends, 2)
	})

	t.Run("duplicate backend not added", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))
		reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))
		got := reg.Lookup("app.localhost")
		require.NotNil(t, got)
		assert.Len(t, got.Backends, 1)
	})

	t.Run("multiple distinct hosts", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))
		reg.Register(NewRoute("api.localhost", mustURL("http://10.0.0.2:3000"), "docker", NewLoadBalancer("")))
		assert.NotNil(t, reg.Lookup("app.localhost"))
		assert.NotNil(t, reg.Lookup("api.localhost"))
	})
}

func TestDeregister(t *testing.T) {
	t.Run("non-existent host is a no-op", func(t *testing.T) {
		reg := NewRegistry()
		assert.NotPanics(t, func() { reg.Deregister("missing.localhost", nil) })
	})

	t.Run("nil target removes entire route", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))
		reg.Deregister("app.localhost", nil)
		assert.Nil(t, reg.Lookup("app.localhost"))
	})

	t.Run("single backend with target removes entire route", func(t *testing.T) {
		reg := NewRegistry()
		target := mustURL("http://10.0.0.1:3000")
		reg.Register(NewRoute("app.localhost", target, "docker", NewLoadBalancer("")))
		reg.Deregister("app.localhost", target)
		assert.Nil(t, reg.Lookup("app.localhost"))
	})

	t.Run("removes specific backend from multi-backend route", func(t *testing.T) {
		reg := NewRegistry()
		t1 := mustURL("http://10.0.0.1:3000")
		t2 := mustURL("http://10.0.0.2:3000")
		r := NewRoute("app.localhost", t1, "docker", NewLoadBalancer(""))
		r.AddBackend(NewBackend(t2))
		reg.Register(r)

		reg.Deregister("app.localhost", t1)

		got := reg.Lookup("app.localhost")
		require.NotNil(t, got)
		require.Len(t, got.Backends, 1)
		assert.Equal(t, t2.String(), got.Backends[0].Target.String())
	})

	t.Run("deregistering unknown target on existing route is a no-op", func(t *testing.T) {
		reg := NewRegistry()
		t1 := mustURL("http://10.0.0.1:3000")
		t2 := mustURL("http://10.0.0.2:3000")
		r := NewRoute("app.localhost", t1, "docker", NewLoadBalancer(""))
		r.AddBackend(NewBackend(t2))
		reg.Register(r)

		reg.Deregister("app.localhost", mustURL("http://99.99.99.99:9999"))

		got := reg.Lookup("app.localhost")
		require.NotNil(t, got)
		assert.Len(t, got.Backends, 2)
	})
}

func TestLookup(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))

	t.Run("existing host", func(t *testing.T) {
		got := reg.Lookup("app.localhost")
		require.NotNil(t, got)
		assert.Equal(t, "app.localhost", got.Host)
	})

	t.Run("missing host returns nil", func(t *testing.T) {
		assert.Nil(t, reg.Lookup("missing.localhost"))
	})
}

func TestHostsBySource(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))
	reg.Register(NewRoute("api.localhost", mustURL("http://10.0.0.2:3000"), "docker", NewLoadBalancer("")))
	reg.Register(NewRoute("svc.localhost", mustURL("http://10.0.0.3:3000"), "kubernetes", NewLoadBalancer("")))

	assert.ElementsMatch(t, []string{"app.localhost", "api.localhost"}, reg.HostsBySource("docker"))
	assert.ElementsMatch(t, []string{"svc.localhost"}, reg.HostsBySource("kubernetes"))
	assert.Empty(t, reg.HostsBySource("unknown"))
}

func TestHosts(t *testing.T) {
	t.Run("empty registry", func(t *testing.T) {
		reg := NewRegistry()
		assert.Empty(t, reg.Hosts())
	})

	t.Run("returns all hosts", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(NewRoute("app.localhost", mustURL("http://10.0.0.1:3000"), "docker", NewLoadBalancer("")))
		reg.Register(NewRoute("api.localhost", mustURL("http://10.0.0.2:3000"), "docker", NewLoadBalancer("")))
		assert.ElementsMatch(t, []string{"app.localhost", "api.localhost"}, reg.Hosts())
	})
}

func TestRandomLoadBalancer(t *testing.T) {
	t.Run("empty backends returns nil", func(t *testing.T) {
		lb := NewRandomLoadBalancer()
		assert.Nil(t, lb.Pick([]*Backend{}))
	})

	t.Run("picks from non-empty backends", func(t *testing.T) {
		lb := NewRandomLoadBalancer()
		b := &Backend{Target: mustURL("http://10.0.0.1:3000")}
		assert.Equal(t, b, lb.Pick([]*Backend{b}))
	})
}

func TestRoundRobinLoadBalancer(t *testing.T) {
	t.Run("empty backends returns nil", func(t *testing.T) {
		lb := NewRoundRobinLoadBalancer()
		assert.Nil(t, lb.Pick([]*Backend{}))
	})

	t.Run("cycles through backends in order", func(t *testing.T) {
		lb := NewRoundRobinLoadBalancer()
		b1 := &Backend{Target: mustURL("http://10.0.0.1:3000")}
		b2 := &Backend{Target: mustURL("http://10.0.0.2:3000")}
		backends := []*Backend{b1, b2}

		assert.Equal(t, b1, lb.Pick(backends))
		assert.Equal(t, b2, lb.Pick(backends))
		assert.Equal(t, b1, lb.Pick(backends))
	})
}
