// Package kubernetes
package kubernetes

import (
	"context"
	"log/slog"

	"cloud-native-reverse-proxy/pkg/provider"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	netv1listers "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
)

type ingressInformer interface {
	AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error)
	Run(stopCh <-chan struct{})
	HasSynced() bool
}

type ingressLister interface {
	List(selector labels.Selector) ([]*netv1.Ingress, error)
}

type Provider struct {
	informer     ingressInformer
	lister       ingressLister
	name         string
	ingressClass string
}

var (
	_ ingressInformer   = cache.SharedIndexInformer(nil)
	_ ingressLister     = netv1listers.IngressLister(nil)
	_ provider.Provider = (*Provider)(nil)
)

func New(name string, informer ingressInformer, lister ingressLister, ingressClass string) *Provider {
	return &Provider{informer: informer, lister: lister, name: name, ingressClass: ingressClass}
}

func (kp *Provider) Name() string { return kp.name }

func (kp *Provider) Watch(ctx context.Context, watcherBuffer chan<- provider.Event, logger *slog.Logger) error {
	return nil
}
