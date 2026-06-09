// Package kubernetes
package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/url"
	"slices"
	"strconv"
	"time"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	typednetv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
)

// Signals the watch stream ended and needs to be restarted
var errWatchClosed = errors.New("watch closed")

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
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	innerBuffer := make(chan watch.Event, innerBufferSize)
	go kp.processEvents(watchCtx, innerBuffer, watcherBuffer, logger)

	for {
		listCtx, cancel := context.WithTimeout(watchCtx, listTimeout)
		list, err := kp.client.List(listCtx, metav1.ListOptions{})
		cancel()
		if err != nil {
			return fmt.Errorf("ingress list failed: %w", err)
		}

		w, err := kp.client.Watch(watchCtx, metav1.ListOptions{ResourceVersion: list.ResourceVersion})
		if err != nil {
			return fmt.Errorf("ingress watch failed: %w", err)
		}

		err = kp.drainWatch(watchCtx, w, innerBuffer, logger)

		// Reconnect if connection lost
		if errors.Is(err, errWatchClosed) {
			logger.Warn("ingress watch closed, reconnecting")
			continue
		}
		if err != nil {
			return err
		}
	}
}

func (kp *Provider) processEvents(ctx context.Context, innerBuffer <-chan watch.Event, watcherBuffer chan<- provider.Event, logger *slog.Logger) {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	// Initial batch load on proxy connect
	kp.reconcile(ctx, watcherBuffer, logger)

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-innerBuffer:
			ingress, ok := event.Object.(*netv1.Ingress)
			if !ok {
				logger.Warn("unexpected object type in watch event", "type", event.Type)
				continue
			}
			switch event.Type {
			case watch.Added, watch.Modified:
				kp.emitRegister(ctx, ingress, watcherBuffer)
			case watch.Deleted:
				kp.emitDeregister(ctx, ingress, watcherBuffer)
			}
		case <-ticker.C:
			kp.reconcile(ctx, watcherBuffer, logger)
		}
	}
}

func (kp *Provider) reconcile(ctx context.Context, watcherBuffer chan<- provider.Event, logger *slog.Logger) {
	listCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	list, err := kp.client.List(listCtx, metav1.ListOptions{})
	if err != nil {
		logger.Error("reconcile list failed", "err", err)
		return
	}
	kp.emitBatch(ctx, watcherBuffer, list.Items)
}

func (kp *Provider) drainWatch(ctx context.Context, w watch.Interface, innerBuffer chan<- watch.Event, logger *slog.Logger) error {
	defer w.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-w.ResultChan():
			if !ok {
				return errWatchClosed
			}
			if event.Type == watch.Error {
				logger.Warn("ingress watch error event", "object", event.Object)
				return errWatchClosed
			}
			if !trySend(innerBuffer, event) {
				logger.Warn("inner buffer full, dropping event",
					"type", event.Type, "depth", len(innerBuffer), "cap", cap(innerBuffer))
			}
		}
	}
}

func trySend(innerBuffer chan<- watch.Event, event watch.Event) bool {
	select {
	case innerBuffer <- event:
		return true
	default:
		return false
	}
}

func (kp *Provider) emitBatch(ctx context.Context, watcherBuffer chan<- provider.Event, ingresses []netv1.Ingress) {
	routeMap := make(map[string]*registry.Route)
	for i := range ingresses {
		routes := kp.parseRoute(&ingresses[i])
		for _, route := range routes {
			if existing, ok := routeMap[route.Host]; ok {
				existing.AddBackend(route.Backends[0])
			} else {
				routeMap[route.Host] = route
			}
		}
	}
	emit(ctx, watcherBuffer, provider.BatchChange{Source: kp.name, Routes: slices.Collect(maps.Values(routeMap))})
}

func (kp *Provider) emitRegister(ctx context.Context, ingress *netv1.Ingress, watcherBuffer chan<- provider.Event) {
	for _, route := range kp.parseRoute(ingress) {
		emit(ctx, watcherBuffer, provider.Change{Op: provider.OpRegister, Host: route.Host, Route: route})
	}
}

func (kp *Provider) emitDeregister(ctx context.Context, ingress *netv1.Ingress, watcherBuffer chan<- provider.Event) {
	for _, route := range kp.parseRoute(ingress) {
		emit(ctx, watcherBuffer, provider.Change{Op: provider.OpDeregister, Host: route.Host, Route: route})
	}
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
