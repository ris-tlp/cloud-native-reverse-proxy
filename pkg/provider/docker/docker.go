package docker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"time"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

var errSkipContainer = errors.New("container not configured for proxy")

const (
	inspectTimeout    = 5 * time.Second
	reconcileInterval = 30 * time.Second
	innerBufferSize   = 100
	HostLabel         = "proxy.host"
	PortLabel         = "proxy.port"
	LoadBalancerLabel = "proxy.loadbalancer"
)

var _ provider.Provider = (*Provider)(nil)

type dockerClient interface {
	Events(ctx context.Context, options client.EventsListOptions) client.EventsResult
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

var _ dockerClient = (*client.Client)(nil)

type Provider struct {
	client dockerClient
	name   string
}

func New(name string, cli dockerClient) *Provider {
	return &Provider{client: cli, name: name}
}

func (dp *Provider) Name() string { return dp.name }

func (dp *Provider) Watch(ctx context.Context, watcherBuffer chan<- provider.Event, logger *slog.Logger) error {
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	dockerEvents := dp.client.Events(watchCtx, client.EventsListOptions{})

	innerBuffer := make(chan events.Message, innerBufferSize)
	go dp.processEvents(watchCtx, innerBuffer, watcherBuffer, logger)

	for {
		select {
		case <-watchCtx.Done():
			return watchCtx.Err()
		case err := <-dockerEvents.Err:
			logger.Error("docker events stream error", "err", err)
			return err
		case msg := <-dockerEvents.Messages:
			if !isContainerAction(msg) {
				continue
			}
			if !trySend(innerBuffer, msg) {
				logger.Warn("inner buffer full, dropping event",
					"action", msg.Action, "id", msg.Actor.ID,
					"depth", len(innerBuffer), "cap", cap(innerBuffer))
			}
		}
	}
}

func isContainerAction(msg events.Message) bool {
	if msg.Type != events.ContainerEventType {
		return false
	}
	switch msg.Action {
	case events.ActionStart, events.ActionKill:
		return true
	}
	return false
}

func trySend(innerBuffer chan<- events.Message, msg events.Message) bool {
	select {
	case innerBuffer <- msg:
		return true
	default:
		return false
	}
}

func (dp *Provider) processEvents(ctx context.Context, innerBuffer <-chan events.Message, watcherBuffer chan<- provider.Event, logger *slog.Logger) {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	dp.reconcile(ctx, watcherBuffer, logger)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-innerBuffer:
			switch msg.Action {
			case events.ActionStart:
				dp.emitRegister(ctx, msg.Actor.ID, watcherBuffer, logger)
			case events.ActionKill:
				dp.emitDeregister(ctx, msg.Actor.ID, watcherBuffer, logger)
			}
		case <-ticker.C:
			dp.reconcile(ctx, watcherBuffer, logger)
		}
	}
}

func (dp *Provider) emitRegister(ctx context.Context, containerID string, watcherBuffer chan<- provider.Event, logger *slog.Logger) {
	route, err := dp.inspectRoute(ctx, containerID)
	if errors.Is(err, errSkipContainer) {
		return
	}
	if err != nil {
		logger.Error("inspect route failed", "id", containerID, "err", err)
		return
	}

	emit(ctx, watcherBuffer, provider.Change{Op: provider.OpRegister, Host: route.Host, Route: route})
}

func (dp *Provider) emitDeregister(ctx context.Context, containerID string, watcherBuffer chan<- provider.Event, logger *slog.Logger) {
	route, err := dp.inspectRoute(ctx, containerID)
	if errors.Is(err, errSkipContainer) {
		return
	}
	if err != nil {
		logger.Error("inspect route failed", "id", containerID, "err", err)
		return
	}

	emit(ctx, watcherBuffer, provider.Change{Op: provider.OpDeregister, Host: route.Host, Route: route})
}

func (dp *Provider) reconcile(ctx context.Context, watcherBuffer chan<- provider.Event, logger *slog.Logger) {
	listCtx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()

	list, err := dp.client.ContainerList(listCtx, client.ContainerListOptions{})
	if err != nil {
		logger.Error("reconcile list failed", "err", err)
		return
	}

	routeMap := make(map[string]*registry.Route)
	for _, c := range list.Items {
		route, err := dp.inspectRoute(ctx, c.ID)
		if errors.Is(err, errSkipContainer) {
			continue
		}
		if err != nil {
			logger.Error("inspect route failed", "id", c.ID, "err", err)
			continue
		}
		if existing, ok := routeMap[route.Host]; ok {
			existing.AddBackend(route.Backends[0])
		} else {
			routeMap[route.Host] = route
		}
	}

	emit(ctx, watcherBuffer, provider.BatchChange{Source: dp.name, Routes: slices.Collect(maps.Values(routeMap))})
}

func emit(ctx context.Context, watcherBuffer chan<- provider.Event, e provider.Event) {
	select {
	case watcherBuffer <- e:
	case <-ctx.Done():
	}
}

func (dp *Provider) inspectRoute(ctx context.Context, containerID string) (*registry.Route, error) {
	inspectCtx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	info, err := dp.client.ContainerInspect(inspectCtx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, err
	}
	return parseRoute(info.Container, dp.name)
}

func parseRoute(info container.InspectResponse, source string) (*registry.Route, error) {
	host, ok := info.Config.Labels[HostLabel]
	if !ok {
		return nil, errSkipContainer
	}
	portStr, ok := info.Config.Labels[PortLabel]
	if !ok {
		return nil, errSkipContainer
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	ip, ok := firstValidIP(info.NetworkSettings.Networks)
	if !ok {
		return nil, errors.New("no valid IP on any network")
	}

	targetURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(ip.String(), strconv.FormatUint(port, 10)),
	}

	lb := registry.NewLoadBalancer(info.Config.Labels[LoadBalancerLabel])
	return registry.NewRoute(host, targetURL, source, lb), nil
}

func firstValidIP(networks map[string]*network.EndpointSettings) (netip.Addr, bool) {
	for _, n := range networks {
		if n != nil && n.IPAddress.IsValid() {
			return n.IPAddress, true
		}
	}
	return netip.Addr{}, false
}
