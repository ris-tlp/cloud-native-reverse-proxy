package testutil

import (
	"context"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func NewDockerClient(t *testing.T) *client.Client {
	t.Helper()
	cli, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatalf("failed to create docker client: %v", err)
	}
	return cli
}

func StartContainer(t *testing.T, cli *client.Client, image string, labels map[string]string) string {
	t.Helper()
	ctx := context.Background()

	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Image: image,
		Config: &container.Config{
			Cmd:    []string{"sleep", "infinity"},
			Labels: labels,
		},
	})
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	t.Cleanup(func() {
		_, _ = cli.ContainerRemove(context.Background(), created.ID, client.ContainerRemoveOptions{Force: true})
	})

	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	return created.ID
}
