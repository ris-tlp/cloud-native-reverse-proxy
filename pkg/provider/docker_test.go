package provider

import (
	"context"
	"net/netip"
	"testing"

	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var validNetworks = map[string]*network.EndpointSettings{
	"bridge": {IPAddress: netip.MustParseAddr("172.17.0.2")},
}

// inspect builds an InspectResponse with the given labels and validNetworks.
func inspect(labels map[string]string) container.InspectResponse {
	return container.InspectResponse{
		Config:          &container.Config{Labels: labels},
		NetworkSettings: &container.NetworkSettings{Networks: validNetworks},
	}
}

// proxyLabels builds a label map with the proxy host and port set.
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
