// Package kubernetes
package kubernetes

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strconv"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"

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
	stopCh := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(stopCh)
	}()

	if _, err := kp.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { kp.reconcile(ctx, watcherBuffer, logger) },
		UpdateFunc: func(any, any) { kp.reconcile(ctx, watcherBuffer, logger) },
		DeleteFunc: func(any) { kp.reconcile(ctx, watcherBuffer, logger) },
	}); err != nil {
		return err
	}

	go kp.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, kp.informer.HasSynced) {
		return errors.New("ingress cache sync failed")
	}

	<-ctx.Done()
	return ctx.Err()
}

func (kp *Provider) reconcile(ctx context.Context, watcherBuffer chan<- provider.Event, logger *slog.Logger) {
	ingresses, err := kp.lister.List(labels.Everything())
	if err != nil {
		logger.Error("reconcile list failed", "err", err)
		return
	}

	var routes []*registry.Route
	for _, ingress := range ingresses {
		routes = append(routes, kp.parseRoute(ingress)...)
	}

	emit(ctx, watcherBuffer, provider.BatchChange{Source: kp.name, Routes: routes})
}

func emit(ctx context.Context, watcherBuffer chan<- provider.Event, e provider.Event) {
	select {
	case watcherBuffer <- e:
	case <-ctx.Done():
	}
}

func (kp *Provider) parseRoute(ingress *netv1.Ingress) []*registry.Route {
	if ingress.Spec.IngressClassName == nil || *ingress.Spec.IngressClassName != kp.ingressClass {
		return nil
	}

	var routes []*registry.Route
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			svc := path.Backend.Service
			if svc == nil {
				continue
			}
			target := serviceURL(svc.Name, ingress.Namespace, svc.Port.Number)
			lb := registry.NewLoadBalancer("")
			routes = append(routes, registry.NewRoute(rule.Host, target, kp.name, lb))
		}
	}
	return routes
}

func serviceURL(name, namespace string, port int32) *url.URL {
	host := name + "." + namespace + ".svc.cluster.local"
	return &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(int(port))),
	}
}
