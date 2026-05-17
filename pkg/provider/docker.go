package provider

import (
	"context"
	"fmt"
	"log"

	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

const DockerName = "docker"

type DockerProvider struct {
	providerName string
}

func NewDockerProvider() *DockerProvider {
	return &DockerProvider{
		providerName: DockerName,
	}
}

func (dp *DockerProvider) Watch(reg *registry.Registry) error {
	ctx := context.Background()
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		log.Fatal(err)
		return err
	}

	dockerEvents := dockerClient.Events(ctx, client.EventsListOptions{})

	for {
		select {
		case err := <-dockerEvents.Err:
			fmt.Println(err)
			return err
		case eventMessage := <-dockerEvents.Messages:

			if eventMessage.Type == events.ContainerEventType {
				switch eventMessage.Action {
				case events.ActionStart:
					dp.handleStart(eventMessage)

				case events.ActionStop, events.ActionDie:
					dp.handleStop(eventMessage)
				}
			}
		}
	}
}

func (dp *DockerProvider) handleStart(eventMessage events.Message) {
	fmt.Println(eventMessage.Action, eventMessage.Actor.ID, eventMessage.Type)
}

func (dp *DockerProvider) handleStop(eventsMessage events.Message) {
	fmt.Println(eventsMessage.Action, eventsMessage.Actor.ID, eventsMessage.Type)
}
