package provider

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"cloud-native-reverse-proxy/internal/testutil"
	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDockerClient struct {
	dockerClient
	inspectFn func(ctx context.Context, containerID string, opts client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	listFn    func(ctx context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error)
}

func (f *fakeDockerClient) ContainerInspect(ctx context.Context, containerID string, opts client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return f.inspectFn(ctx, containerID, opts)
}

func (f *fakeDockerClient) ContainerList(ctx context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
	return f.listFn(ctx, opts)
}

var validNetworks = map[string]*network.EndpointSettings{
	"bridge": {IPAddress: netip.MustParseAddr("172.17.0.2")},
}

func inspect(labels map[string]string) container.InspectResponse {
	return container.InspectResponse{
		Config:          &container.Config{Labels: labels},
		NetworkSettings: &container.NetworkSettings{Networks: validNetworks},
	}
}

func inspectResult(labels map[string]string) client.ContainerInspectResult {
	return client.ContainerInspectResult{Container: inspect(labels)}
}

func inspectResultWithIP(labels map[string]string, ip string) client.ContainerInspectResult {
	return client.ContainerInspectResult{
		Container: container.InspectResponse{
			Config: &container.Config{Labels: labels},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {IPAddress: netip.MustParseAddr(ip)},
				},
			},
		},
	}
}

func proxyLabels(host, port string) map[string]string {
	return map[string]string{HostLabel: host, PortLabel: port}
}

func TestIsContainerAction(t *testing.T) {
	tests := []struct {
		name string
		msg  events.Message
		want bool
	}{
		{
			name: "start accepted",
			msg:  events.Message{Type: events.ContainerEventType, Action: events.ActionStart},
			want: true,
		},
		{
			name: "kill accepted",
			msg:  events.Message{Type: events.ContainerEventType, Action: events.ActionKill},
			want: true,
		},
		{
			name: "die rejected",
			msg:  events.Message{Type: events.ContainerEventType, Action: events.ActionDie},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isContainerAction(tc.msg))
		})
	}
}

func TestTrySend(t *testing.T) {
	t.Run("sends to non-full channel", func(t *testing.T) {
		ch := make(chan events.Message, 1)
		ok := trySend(ch, events.Message{Action: events.ActionStart})
		assert.True(t, ok)
		assert.Len(t, ch, 1)
	})

	t.Run("drops on full channel", func(t *testing.T) {
		ch := make(chan events.Message, 1)
		ch <- events.Message{}
		ok := trySend(ch, events.Message{Action: events.ActionStart})
		assert.False(t, ok)
		assert.Len(t, ch, 1)
	})
}

func TestEmit(t *testing.T) {
	t.Run("sends when context is active", func(t *testing.T) {
		ch := make(chan Event, 1)
		ev := Change{Op: OpRegister, Host: "app.localhost"}
		emit(context.Background(), ch, ev)
		require.Len(t, ch, 1)
		assert.Equal(t, ev, <-ch)
	})

	t.Run("does not block when context is cancelled", func(t *testing.T) {
		ch := make(chan Event)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		emit(ctx, ch, Change{Op: OpRegister, Host: "app.localhost"})
		assert.Len(t, ch, 0)
	})
}

func TestParseRoute(t *testing.T) {
	tests := []struct {
		name     string
		info     container.InspectResponse
		source   string
		checkErr func(*testing.T, error)
		check    func(*testing.T, *registry.Route)
	}{
		{
			name:     "missing host label",
			info:     inspect(map[string]string{PortLabel: "3000"}),
			source:   "docker",
			checkErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errSkipContainer) },
		},
		{
			name:     "missing port label",
			info:     inspect(map[string]string{HostLabel: "app.localhost"}),
			source:   "docker",
			checkErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errSkipContainer) },
		},
		{
			name:     "empty labels",
			info:     inspect(map[string]string{}),
			source:   "docker",
			checkErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errSkipContainer) },
		},
		{
			name:     "invalid port",
			info:     inspect(proxyLabels("app.localhost", "not-a-port")),
			source:   "docker",
			checkErr: func(t *testing.T, err error) { assert.ErrorContains(t, err, "invalid port") },
		},
		{
			name: "no valid IP",
			info: container.InspectResponse{
				Config:          &container.Config{Labels: proxyLabels("app.localhost", "3000")},
				NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
			},
			source:   "docker",
			checkErr: func(t *testing.T, err error) { assert.ErrorContains(t, err, "no valid IP") },
		},
		{
			name:   "parse route correctly",
			info:   inspect(proxyLabels("app.localhost", "3000")),
			source: "docker",
			check: func(t *testing.T, route *registry.Route) {
				assert.Equal(t, "app.localhost", route.Host)
				assert.Equal(t, "docker", route.Source)
				require.Len(t, route.Backends, 1)
				assert.Equal(t, "172.17.0.2:3000", route.Backends[0].Target.Host)
				assert.Equal(t, "http", route.Backends[0].Target.Scheme)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			route, err := parseRoute(tc.info, tc.source)
			if tc.checkErr != nil {
				require.Error(t, err)
				tc.checkErr(t, err)
				assert.Nil(t, route)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, route)
			if tc.check != nil {
				tc.check(t, route)
			}
		})
	}
}

func TestFirstValidIP(t *testing.T) {
	tests := []struct {
		name     string
		networks map[string]*network.EndpointSettings
		wantIP   netip.Addr
		wantOK   bool
	}{
		{
			name:   "nil map",
			wantOK: false,
		},
		{
			name:     "empty map",
			networks: map[string]*network.EndpointSettings{},
			wantOK:   false,
		},
		{
			name:     "nil endpoint",
			networks: map[string]*network.EndpointSettings{"bridge": nil},
			wantOK:   false,
		},
		{
			name:     "zero IP",
			networks: map[string]*network.EndpointSettings{"bridge": {IPAddress: netip.Addr{}}},
			wantOK:   false,
		},
		{
			name:     "valid IPv4",
			networks: map[string]*network.EndpointSettings{"bridge": {IPAddress: netip.MustParseAddr("172.17.0.2")}},
			wantIP:   netip.MustParseAddr("172.17.0.2"),
			wantOK:   true,
		},
		{
			name: "nil endpoint alongside valid endpoint",
			networks: map[string]*network.EndpointSettings{
				"host":   nil,
				"bridge": {IPAddress: netip.MustParseAddr("172.17.0.5")},
			},
			wantIP: netip.MustParseAddr("172.17.0.5"),
			wantOK: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := firstValidIP(tc.networks)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantIP, got)
			}
		})
	}
}

func TestInspectRoute(t *testing.T) {
	tests := []struct {
		name      string
		inspectFn func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
		checkErr  func(*testing.T, error)
		check     func(*testing.T, *registry.Route)
	}{
		{
			name: "returns route for labelled container",
			inspectFn: func(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				assert.Equal(t, "abc123", id)
				return inspectResult(proxyLabels("app.localhost", "3000")), nil
			},
			check: func(t *testing.T, route *registry.Route) {
				assert.Equal(t, "app.localhost", route.Host)
				assert.Equal(t, "docker", route.Source)
				require.Len(t, route.Backends, 1)
				assert.Equal(t, "172.17.0.2:3000", route.Backends[0].Target.Host)
			},
		},
		{
			name: "propagates inspect error",
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return client.ContainerInspectResult{}, errors.New("connection refused")
			},
			checkErr: func(t *testing.T, err error) { assert.ErrorContains(t, err, "connection refused") },
		},
		{
			name: "returns errSkipContainer for unlabelled container",
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return inspectResult(map[string]string{}), nil
			},
			checkErr: func(t *testing.T, err error) { require.ErrorIs(t, err, errSkipContainer) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dp := NewDockerProvider("docker",&fakeDockerClient{inspectFn: tc.inspectFn})
			route, err := dp.inspectRoute(context.Background(), "abc123")
			if tc.checkErr != nil {
				require.Error(t, err)
				tc.checkErr(t, err)
				assert.Nil(t, route)
				return
			}
			require.NoError(t, err)
			tc.check(t, route)
		})
	}
}

func TestEmitRegister(t *testing.T) {
	tests := []struct {
		name      string
		inspectFn func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
		wantOp    ChangeOp
		wantSent  bool
	}{
		{
			name: "emits register for labelled container",
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return inspectResult(proxyLabels("app.localhost", "3000")), nil
			},
			wantOp:   OpRegister,
			wantSent: true,
		},
		{
			name: "skips unlabelled container",
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return inspectResult(map[string]string{}), nil
			},
			wantSent: false,
		},
		{
			name: "skips on inspect error",
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return client.ContainerInspectResult{}, errors.New("inspect failed")
			},
			wantSent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dp := NewDockerProvider("docker",&fakeDockerClient{inspectFn: tc.inspectFn})
			ch := make(chan Event, 1)
			dp.emitRegister(context.Background(), "abc123", ch, testutil.DiscardLogger())
			if !tc.wantSent {
				assert.Empty(t, ch)
				return
			}
			require.Len(t, ch, 1)
			change, ok := (<-ch).(Change)
			require.True(t, ok)
			assert.Equal(t, tc.wantOp, change.Op)
			assert.Equal(t, "app.localhost", change.Host)
		})
	}
}

func TestEmitDeregister(t *testing.T) {
	tests := []struct {
		name      string
		inspectFn func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
		wantSent  bool
	}{
		{
			name: "emits deregister for labelled container",
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return inspectResult(proxyLabels("app.localhost", "3000")), nil
			},
			wantSent: true,
		},
		{
			name: "skips unlabelled container",
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return inspectResult(map[string]string{}), nil
			},
			wantSent: false,
		},
		{
			name: "skips on inspect error",
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return client.ContainerInspectResult{}, errors.New("inspect failed")
			},
			wantSent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dp := NewDockerProvider("docker",&fakeDockerClient{inspectFn: tc.inspectFn})
			ch := make(chan Event, 1)
			dp.emitDeregister(context.Background(), "abc123", ch, testutil.DiscardLogger())
			if !tc.wantSent {
				assert.Empty(t, ch)
				return
			}
			require.Len(t, ch, 1)
			change, ok := (<-ch).(Change)
			require.True(t, ok)
			assert.Equal(t, OpDeregister, change.Op)
			assert.Equal(t, "app.localhost", change.Host)
		})
	}
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name      string
		listFn    func(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
		inspectFn func(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
		wantSent  bool
		check     func(*testing.T, BatchChange)
	}{
		{
			name: "empty list emits empty batch",
			listFn: func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
				return client.ContainerListResult{}, nil
			},
			wantSent: true,
			check: func(t *testing.T, batch BatchChange) {
				assert.Equal(t, "docker", batch.Source)
				assert.Empty(t, batch.Routes)
			},
		},
		{
			name: "labelled container included in batch",
			listFn: func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
				return client.ContainerListResult{Items: []container.Summary{{ID: "abc"}}}, nil
			},
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return inspectResult(proxyLabels("app.localhost", "3000")), nil
			},
			wantSent: true,
			check: func(t *testing.T, batch BatchChange) {
				require.Len(t, batch.Routes, 1)
				assert.Equal(t, "app.localhost", batch.Routes[0].Host)
			},
		},
		{
			name: "unlabelled container skipped",
			listFn: func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
				return client.ContainerListResult{Items: []container.Summary{{ID: "abc"}}}, nil
			},
			inspectFn: func(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				return inspectResult(map[string]string{}), nil
			},
			wantSent: true,
			check: func(t *testing.T, batch BatchChange) {
				assert.Empty(t, batch.Routes)
			},
		},
		{
			name: "shared host merges backends",
			listFn: func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
				return client.ContainerListResult{Items: []container.Summary{{ID: "a"}, {ID: "b"}}}, nil
			},
			inspectFn: func(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
				ip := map[string]string{"a": "172.17.0.2", "b": "172.17.0.3"}[id]
				return inspectResultWithIP(proxyLabels("app.localhost", "3000"), ip), nil
			},
			wantSent: true,
			check: func(t *testing.T, batch BatchChange) {
				require.Len(t, batch.Routes, 1)
				assert.Len(t, batch.Routes[0].Backends, 2)
			},
		},
		{
			name: "list error sends nothing",
			listFn: func(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
				return client.ContainerListResult{}, errors.New("daemon unreachable")
			},
			wantSent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dp := NewDockerProvider("docker",&fakeDockerClient{
				listFn:    tc.listFn,
				inspectFn: tc.inspectFn,
			})
			ch := make(chan Event, 1)
			dp.reconcile(context.Background(), ch, testutil.DiscardLogger())
			if !tc.wantSent {
				assert.Empty(t, ch)
				return
			}
			require.Len(t, ch, 1)
			batch, ok := (<-ch).(BatchChange)
			require.True(t, ok)
			tc.check(t, batch)
		})
	}
}
