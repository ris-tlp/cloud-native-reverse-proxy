package provider

import (
	"context"
	"fmt"
	"log"

	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/moby/moby/client"
)

const DockerName = "docker"

type DockerProvider struct {
	providerName string
}

func (dp *DockerProvider) NewDockerProvider() *DockerProvider {
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

	events := dockerClient.Events(ctx, client.EventsListOptions{})

	for {
		select {
		case err := <-events.Err:
			fmt.Println(err)
			return err
		case eventMessage := <-events.Messages:
			fmt.Println(eventMessage)
		}
	}
}
