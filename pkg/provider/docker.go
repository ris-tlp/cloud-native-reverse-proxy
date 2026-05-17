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

	for {
		select {
		case err := <-dockerEvents.Err:
			fmt.Println(err)
			return err
		case eventMessage := <-dockerEvents.Messages:

			if eventMessage.Type == events.ContainerEventType {
				switch eventMessage.Action {
				case events.ActionStart:
					dp.handleStart(ctx, eventMessage, reg)

				case events.ActionStop, events.ActionDie:
					dp.handleStop(eventMessage)
				}
			}
		}
	}
}

func (dp *DockerProvider) handleStart(ctx context.Context, eventMessage events.Message, reg *registry.Registry) {
	fmt.Println(eventMessage.Action, eventMessage.Actor.ID, eventMessage.Type)

	containerInfo, err := dp.client.ContainerInspect(ctx, eventMessage.Actor.ID, client.ContainerInspectOptions{})
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
		fmt.Println("no IP found for container", eventMessage.Actor.ID)
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

func (dp *DockerProvider) handleStop(eventsMessage events.Message) {
	fmt.Println(eventsMessage.Action, eventsMessage.Actor.ID, eventsMessage.Type)
}
