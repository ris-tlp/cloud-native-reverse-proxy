// Package kubernetes
package kubernetes

import (
	"context"
	"log/slog"
	"time"

	"cloud-native-reverse-proxy/pkg/provider"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	typednetv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
)

const (
	reconcileInterval = 30 * time.Second
	innerBufferSize   = 100
	listTimeout       = 5 * time.Second
)

type ingressClient interface {
	List(ctx context.Context, opts metav1.ListOptions) (*netv1.IngressList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

type Provider struct {
	client       ingressClient
	name         string
	ingressClass string
}

var (
	_ ingressClient     = typednetv1.IngressInterface(nil)
	_ provider.Provider = (*Provider)(nil)
)

func New(name string, client ingressClient, ingressClass string) *Provider {
	return &Provider{client: client, name: name, ingressClass: ingressClass}
}

func (kp *Provider) Name() string { return kp.name }

func (kp *Provider) Watch(ctx context.Context, watcherBuffer chan<- provider.Event, logger *slog.Logger) error {
	return nil
}
