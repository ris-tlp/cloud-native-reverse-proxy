package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

var errSkipContainer = errors.New("container not configured for proxy")

const (
	inspectTimeout  = 5 * time.Second
	innerBufferSize = 100
	HostLabel       = "proxy.host"
	PortLabel       = "proxy.port"
)

var _ Provider = (*DockerProvider)(nil)

type DockerProvider struct {
	client *client.Client
	name   string
}

func NewDockerProvider(name string) (*DockerProvider, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &DockerProvider{client: cli, name: name}, nil
}

func (dp *DockerProvider) Name() string { return dp.name }

func (dp *DockerProvider) Watch(ctx context.Context, changes chan<- Change, logger *slog.Logger) error {
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	dockerEvents := dp.client.Events(watchCtx, client.EventsListOptions{})

	innerBuffer := make(chan events.Message, innerBufferSize)
	go dp.processEvents(watchCtx, innerBuffer, changes, logger)

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
			// Will drop backpressured event and eventually reconcile
			if !trySend(innerBuffer, msg) {
				logger.Warn("inner buffer full, dropping event",
					"action", msg.Action, "id", msg.Actor.ID,
					"depth", len(innerBuffer), "cap", cap(innerBuffer))
			}
		}
	}
}

// Only react to start, stop, and die container events
func isContainerAction(msg events.Message) bool {
	if msg.Type != events.ContainerEventType {
		return false
	}
	switch msg.Action {
	case events.ActionStart, events.ActionStop, events.ActionDie:
		return true
	}
	return false
}

// Attempt sending event to internal buffer, will fail if backpressured
func trySend(innerBuffer chan<- events.Message, msg events.Message) bool {
	select {
	case innerBuffer <- msg:
		return true
	default:
		return false
	}
}

// Reads filtered container events from internal buffer and sends out Change events to watcher channel
func (dp *DockerProvider) processEvents(ctx context.Context, innerBuffer <-chan events.Message, watcherBuffer chan<- Change, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-innerBuffer:
			switch msg.Action {
			case events.ActionStart:
				dp.emitRegister(ctx, msg.Actor.ID, watcherBuffer, logger)
			case events.ActionStop, events.ActionDie:
				dp.emitDeregister(ctx, msg.Actor.ID, watcherBuffer, logger)
			}
		}
	}
}

func (dp *DockerProvider) emitRegister(ctx context.Context, containerID string, watcherBuffer chan<- Change, logger *slog.Logger) {
	inspectCtx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	info, err := dp.client.ContainerInspect(inspectCtx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		logger.Error("inspect failed", "id", containerID, "err", err)
		return
	}

	route, err := parseRoute(info.Container, dp.name)
	if errors.Is(err, errSkipContainer) {
		return
	}
	if err != nil {
		logger.Error("parse route failed", "id", containerID, "err", err)
		return
	}

	emit(ctx, watcherBuffer, Change{Op: OpRegister, Source: dp.name, Host: route.Host, Route: route})
}

func (dp *DockerProvider) emitDeregister(ctx context.Context, containerID string, watcherBuffer chan<- Change, logger *slog.Logger) {
	inspectCtx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	info, err := dp.client.ContainerInspect(inspectCtx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		logger.Error("inspect failed", "id", containerID, "err", err)
		return
	}

	host, ok := info.Container.Config.Labels[HostLabel]
	if !ok {
		return
	}

	emit(ctx, watcherBuffer, Change{Op: OpDeregister, Source: dp.name, Host: host})
}

// emit performs a cancellable blocking send to the framework channel.
func emit(ctx context.Context, watcherBuffer chan<- Change, c Change) {
	select {
	case watcherBuffer <- c:
	case <-ctx.Done():
	}
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

	return registry.NewRoute(host, targetURL, source), nil
}

func firstValidIP(networks map[string]*network.EndpointSettings) (netip.Addr, bool) {
	for _, n := range networks {
		if n != nil && n.IPAddress.IsValid() {
			return n.IPAddress, true
		}
	}
	return netip.Addr{}, false
}
