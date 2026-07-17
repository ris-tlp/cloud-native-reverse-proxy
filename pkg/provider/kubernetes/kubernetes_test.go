package kubernetes

import (
	"context"
	"testing"

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
