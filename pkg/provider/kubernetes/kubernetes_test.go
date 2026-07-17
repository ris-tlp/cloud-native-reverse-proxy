package kubernetes

import (
	"context"
	"errors"
	"testing"

	"cloud-native-reverse-proxy/internal/testutil"
	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type fakeIngressClient struct {
	listFn  func(ctx context.Context, opts metav1.ListOptions) (*netv1.IngressList, error)
	watchFn func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

var _ ingressClient = (*fakeIngressClient)(nil)

func (f *fakeIngressClient) List(ctx context.Context, opts metav1.ListOptions) (*netv1.IngressList, error) {
	return f.listFn(ctx, opts)
}

func (f *fakeIngressClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return f.watchFn(ctx, opts)
}

type fakeWatch struct {
	ch      chan watch.Event
	stopped bool
}

var _ watch.Interface = (*fakeWatch)(nil)

func newFakeWatch(buffer int) *fakeWatch {
	return &fakeWatch{ch: make(chan watch.Event, buffer)}
}

func (f *fakeWatch) ResultChan() <-chan watch.Event { return f.ch }

func (f *fakeWatch) Stop() {
	if !f.stopped {
		f.stopped = true
		close(f.ch)
	}
}

type rule struct {
	host string
	svc  string
	port int32
}

func ingress(class, namespace string, rules ...rule) *netv1.Ingress {
	ing := &netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}
	if class != "" {
		ing.Spec.IngressClassName = &class
	}
	for _, r := range rules {
		ing.Spec.Rules = append(ing.Spec.Rules, netv1.IngressRule{
			Host: r.host,
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: []netv1.HTTPIngressPath{{
						Backend: netv1.IngressBackend{
							Service: &netv1.IngressServiceBackend{
								Name: r.svc,
								Port: netv1.ServiceBackendPort{Number: r.port},
							},
						},
					}},
				},
			},
		})
	}
	return ing
}

func TestParseRoute(t *testing.T) {
	tests := []struct {
		name    string
		ingress *netv1.Ingress
		check   func(*testing.T, []*registry.Route)
	}{
		{
			name:    "matching class yields a route",
			ingress: ingress("cnrp", "default", rule{"app.localhost", "web", 8080}),
			check: func(t *testing.T, routes []*registry.Route) {
				require.Len(t, routes, 1)
				assert.Equal(t, "app.localhost", routes[0].Host)
				assert.Equal(t, "kubernetes", routes[0].Source)
				require.Len(t, routes[0].Backends, 1)
				assert.Equal(t, "http", routes[0].Backends[0].Target.Scheme)
				assert.Equal(t, "web.default.svc.cluster.local:8080", routes[0].Backends[0].Target.Host)
			},
		},
		{
			name:    "wrong class is filtered out",
			ingress: ingress("other", "default", rule{"app.localhost", "web", 8080}),
			check: func(t *testing.T, routes []*registry.Route) {
				assert.Empty(t, routes)
			},
		},
		{
			name:    "nil class is filtered out",
			ingress: ingress("", "default", rule{"app.localhost", "web", 8080}),
			check: func(t *testing.T, routes []*registry.Route) {
				assert.Empty(t, routes)
			},
		},
		{
			name: "multiple rules yield multiple routes",
			ingress: ingress(
				"cnrp", "default",
				rule{"a.localhost", "svc-a", 80},
				rule{"b.localhost", "svc-b", 90},
			),
			check: func(t *testing.T, routes []*registry.Route) {
				require.Len(t, routes, 2)
				hosts := []string{routes[0].Host, routes[1].Host}
				assert.ElementsMatch(t, []string{"a.localhost", "b.localhost"}, hosts)
			},
		},
		{
			name: "rule without HTTP block is skipped",
			ingress: func() *netv1.Ingress {
				class := "cnrp"
				return &netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
					Spec: netv1.IngressSpec{
						IngressClassName: &class,
						Rules:            []netv1.IngressRule{{Host: "app.localhost"}},
					},
				}
			}(),
			check: func(t *testing.T, routes []*registry.Route) {
				assert.Empty(t, routes)
			},
		},
		{
			name: "path without a service is skipped",
			ingress: func() *netv1.Ingress {
				class := "cnrp"
				return &netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
					Spec: netv1.IngressSpec{
						IngressClassName: &class,
						Rules: []netv1.IngressRule{{
							Host: "app.localhost",
							IngressRuleValue: netv1.IngressRuleValue{
								HTTP: &netv1.HTTPIngressRuleValue{
									Paths: []netv1.HTTPIngressPath{{
										Backend: netv1.IngressBackend{Service: nil},
									}},
								},
							},
						}},
					},
				}
			}(),
			check: func(t *testing.T, routes []*registry.Route) {
				assert.Empty(t, routes)
			},
		},
	}

	kp := New("kubernetes", nil, "cnrp")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, kp.parseRoute(tc.ingress))
		})
	}
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name     string
		listFn   func(context.Context, metav1.ListOptions) (*netv1.IngressList, error)
		wantSent bool
		check    func(*testing.T, provider.BatchChange)
	}{
		{
			name: "empty list emits empty batch",
			listFn: func(_ context.Context, _ metav1.ListOptions) (*netv1.IngressList, error) {
				return &netv1.IngressList{}, nil
			},
			wantSent: true,
			check: func(t *testing.T, batch provider.BatchChange) {
				assert.Equal(t, "kubernetes", batch.Source)
				assert.Empty(t, batch.Routes)
			},
		},
		{
			name: "matching ingress included in batch",
			listFn: func(_ context.Context, _ metav1.ListOptions) (*netv1.IngressList, error) {
				return &netv1.IngressList{Items: []netv1.Ingress{
					*ingress("cnrp", "default", rule{"app.localhost", "web", 8080}),
				}}, nil
			},
			wantSent: true,
			check: func(t *testing.T, batch provider.BatchChange) {
				require.Len(t, batch.Routes, 1)
				assert.Equal(t, "app.localhost", batch.Routes[0].Host)
			},
		},
		{
			name: "wrong class ingress skipped",
			listFn: func(_ context.Context, _ metav1.ListOptions) (*netv1.IngressList, error) {
				return &netv1.IngressList{Items: []netv1.Ingress{
					*ingress("other", "default", rule{"app.localhost", "web", 8080}),
				}}, nil
			},
			wantSent: true,
			check: func(t *testing.T, batch provider.BatchChange) {
				assert.Empty(t, batch.Routes)
			},
		},
		{
			name: "shared host across ingresses merges backends",
			listFn: func(_ context.Context, _ metav1.ListOptions) (*netv1.IngressList, error) {
				return &netv1.IngressList{Items: []netv1.Ingress{
					*ingress("cnrp", "default", rule{"app.localhost", "web-a", 8080}),
					*ingress("cnrp", "default", rule{"app.localhost", "web-b", 8080}),
				}}, nil
			},
			wantSent: true,
			check: func(t *testing.T, batch provider.BatchChange) {
				require.Len(t, batch.Routes, 1)
				assert.Len(t, batch.Routes[0].Backends, 2)
			},
		},
		{
			name: "list error sends nothing",
			listFn: func(_ context.Context, _ metav1.ListOptions) (*netv1.IngressList, error) {
				return nil, errors.New("apiserver unreachable")
			},
			wantSent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kp := New("kubernetes", &fakeIngressClient{listFn: tc.listFn}, "cnrp")
			ch := make(chan provider.Event, 1)
			kp.reconcile(context.Background(), ch, testutil.DiscardLogger())
			if !tc.wantSent {
				assert.Empty(t, ch)
				return
			}
			require.Len(t, ch, 1)
			batch, ok := (<-ch).(provider.BatchChange)
			require.True(t, ok)
			tc.check(t, batch)
		})
	}
}
