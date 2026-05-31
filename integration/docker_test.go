//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"cloud-native-reverse-proxy/internal/testutil"
	"cloud-native-reverse-proxy/pkg/provider"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testImage = "alpine"

func TestMain(m *testing.M) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		panic("failed to create docker client: " + err.Error())
	}
	resp, err := cli.ImagePull(context.Background(), testImage, client.ImagePullOptions{})
	if err != nil {
		panic("failed to pull " + testImage + ": " + err.Error())
	}
	if err := resp.Wait(context.Background()); err != nil {
		panic("image pull failed: " + err.Error())
	}
	m.Run()
}

func newDockerProvider(t *testing.T) *provider.DockerProvider {
	t.Helper()
	dp, err := provider.NewDockerProvider("docker")
	require.NoError(t, err)
	return dp
}

func runWatch(t *testing.T, dp *provider.DockerProvider) <-chan provider.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	ch := make(chan provider.Event, 20)
	go dp.Watch(ctx, ch, testutil.DiscardLogger()) //nolint:errcheck
	return ch
}

func TestDockerProvider_InitialSync(t *testing.T) {
	cli := testutil.NewDockerClient(t)
	testutil.StartContainer(t, cli, testImage, map[string]string{provider.HostLabel: "app.localhost", provider.PortLabel: "3000"})
	testutil.StartContainer(t, cli, testImage, map[string]string{provider.HostLabel: "api.localhost", provider.PortLabel: "8080"})

	ch := runWatch(t, newDockerProvider(t))

	ev := testutil.WaitForEvent(t, ch, 5*time.Second, func(ev provider.Event) bool {
		_, ok := ev.(provider.BatchChange)
		return ok
	})

	batch := ev.(provider.BatchChange)
	hosts := make(map[string]bool)
	for _, r := range batch.Routes {
		hosts[r.Host] = true
	}
	assert.True(t, hosts["app.localhost"])
	assert.True(t, hosts["api.localhost"])
}

func TestDockerProvider_ContainerStart(t *testing.T) {
	cli := testutil.NewDockerClient(t)
	ch := runWatch(t, newDockerProvider(t))

	testutil.WaitForEvent(t, ch, 5*time.Second, func(ev provider.Event) bool {
		_, ok := ev.(provider.BatchChange)
		return ok
	})

	testutil.StartContainer(t, cli, testImage, map[string]string{provider.HostLabel: "newapp.localhost", provider.PortLabel: "4000"})

	ev := testutil.WaitForEvent(t, ch, 5*time.Second, func(ev provider.Event) bool {
		c, ok := ev.(provider.Change)
		return ok && c.Op == provider.OpRegister && c.Host == "newapp.localhost"
	})

	change := ev.(provider.Change)
	assert.Equal(t, provider.OpRegister, change.Op)
	require.Len(t, change.Route.Backends, 1)
	assert.NotEmpty(t, change.Route.Backends[0].Target.Host)
}

func TestDockerProvider_ContainerKill(t *testing.T) {
	cli := testutil.NewDockerClient(t)
	id := testutil.StartContainer(t, cli, testImage, map[string]string{provider.HostLabel: "killme.localhost", provider.PortLabel: "5000"})

	ch := runWatch(t, newDockerProvider(t))

	testutil.WaitForEvent(t, ch, 5*time.Second, func(ev provider.Event) bool {
		_, ok := ev.(provider.BatchChange)
		return ok
	})

	_, err := cli.ContainerKill(context.Background(), id, client.ContainerKillOptions{})
	require.NoError(t, err)

	ev := testutil.WaitForEvent(t, ch, 5*time.Second, func(ev provider.Event) bool {
		c, ok := ev.(provider.Change)
		return ok && c.Op == provider.OpDeregister && c.Host == "killme.localhost"
	})

	change := ev.(provider.Change)
	assert.Equal(t, provider.OpDeregister, change.Op)
	assert.Equal(t, "killme.localhost", change.Host)
}

func TestDockerProvider_ReconcileMergesSharedHost(t *testing.T) {
	cli := testutil.NewDockerClient(t)
	testutil.StartContainer(t, cli, testImage, map[string]string{provider.HostLabel: "shared.localhost", provider.PortLabel: "3000"})
	testutil.StartContainer(t, cli, testImage, map[string]string{provider.HostLabel: "shared.localhost", provider.PortLabel: "3000"})

	ch := runWatch(t, newDockerProvider(t))

	ev := testutil.WaitForEvent(t, ch, 5*time.Second, func(ev provider.Event) bool {
		_, ok := ev.(provider.BatchChange)
		return ok
	})

	batch := ev.(provider.BatchChange)
	for _, r := range batch.Routes {
		if r.Host == "shared.localhost" {
			assert.Len(t, r.Backends, 2)
			return
		}
	}
	t.Fatal("shared.localhost not found in batch")
}
