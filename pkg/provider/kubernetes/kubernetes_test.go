package kubernetes

import (
	"context"

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
