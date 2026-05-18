package provider

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"time"

	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/cenkalti/backoff/v4"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

var errSkipContainer = errors.New("container not configured for proxy")

const watchMaxElapsed = 10 * time.Minute

const (
	HostLabel = "proxy.host"
	PortLabel = "proxy.port"
)

type DockerProvider struct {
	client *client.Client
}

func NewDockerProvider() (*DockerProvider, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &DockerProvider{
		client: cli,
	}, nil
}

func (dp *DockerProvider) Watch(reg *registry.Registry) error {
	b := backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(watchMaxElapsed))
	return backoff.Retry(func() error {
		return dp.watchOnce(reg)
	}, b)
}

func (dp *DockerProvider) watchOnce(reg *registry.Registry) error {
	ctx := context.Background()

	dockerEvents := dp.client.Events(ctx, client.EventsListOptions{})

	containers, err := dp.client.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		return err
	}

	// Register existing containers
	for _, c := range containers.Items {
		// Add check to ensure container needs to be registered through config
		dp.registerContainer(ctx, c.ID, reg)
	}

	for {
		select {
		case err := <-dockerEvents.Err:
			fmt.Println(err)
			return err
		case eventMessage := <-dockerEvents.Messages:

			if eventMessage.Type == events.ContainerEventType {
				switch eventMessage.Action {
				case events.ActionStart:
					dp.registerContainer(ctx, eventMessage.Actor.ID, reg)

				case events.ActionStop, events.ActionDie:
					dp.deregisterContainer(ctx, eventMessage.Actor.ID, reg)
				}
			}
		}
	}
}

func (dp *DockerProvider) registerContainer(ctx context.Context, containerID string, reg *registry.Registry) {
	containerInfo, err := dp.client.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		fmt.Println("inspect error:", err)
		return
	}

	route, err := parseRoute(containerInfo.Container)
	if errors.Is(err, errSkipContainer) {
		return
	}
	if err != nil {
		fmt.Println("parse error for", containerID, ":", err)
		return
	}

	reg.Register(route)
	fmt.Println("registered route:", route.Host, "->", route.URL)
}

func parseRoute(info container.InspectResponse) (*registry.Route, error) {
	host, ok := info.Config.Labels[HostLabel]
	if !ok {
		return nil, errSkipContainer
	}
	port, ok := info.Config.Labels[PortLabel]
	if !ok {
		return nil, errSkipContainer
	}

	ip, ok := firstValidIP(info.NetworkSettings.Networks)
	if !ok {
		return nil, errors.New("no valid IP on any network")
	}

	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%s", ip, port))
	if err != nil {
		return nil, fmt.Errorf("build target URL: %w", err)
	}

	return registry.NewRoute(host, targetURL), nil
}

func firstValidIP(networks map[string]*network.EndpointSettings) (netip.Addr, bool) {
	for _, n := range networks {
		if n != nil && n.IPAddress.IsValid() {
			return n.IPAddress, true
		}
	}
	return netip.Addr{}, false
}

func (dp *DockerProvider) deregisterContainer(ctx context.Context, containerID string, reg *registry.Registry) {
	containerInfo, err := dp.client.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		fmt.Println("inspect error:", err)
		return
	}

	host, ok := containerInfo.Container.Config.Labels[HostLabel]
	if !ok {
		return
	}

	reg.Deregister(host)
	fmt.Println("deregistered route:", host)
}
