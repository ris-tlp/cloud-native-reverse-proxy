package provider

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"

	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

const DockerName = "docker"

const (
	HostLabel = "proxy.host"
	PortLabel = "proxy.port"
)

type DockerProvider struct {
	providerName string
	client       *client.Client
}

func NewDockerProvider() (*DockerProvider, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &DockerProvider{
		providerName: DockerName,
		client:       cli,
	}, nil
}

func (dp *DockerProvider) Watch(reg *registry.Registry) error {
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

	host, ok := containerInfo.Container.Config.Labels[HostLabel]
	if !ok {
		return
	}
	port, ok := containerInfo.Container.Config.Labels[PortLabel]
	if !ok {
		return
	}

	var containerIP netip.Addr
	for _, network := range containerInfo.Container.NetworkSettings.Networks {
		if network != nil && network.IPAddress.IsValid() {
			containerIP = network.IPAddress
			break
		}
	}

	if !containerIP.IsValid() {
		fmt.Println("no IP found for container", containerID)
		return
	}

	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%s", containerIP, port))
	if err != nil {
		fmt.Println("url parse error:", err)
		return
	}

	reg.Register(registry.NewRoute(host, targetURL))
	fmt.Println("registered route:", host, "->", targetURL.String())
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
