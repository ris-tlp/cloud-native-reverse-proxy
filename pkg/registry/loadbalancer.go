package registry

import (
	"math/rand/v2"
	"sync/atomic"
)

type LoadBalancer interface {
	Pick(backends []*Backend) *Backend
}

func NewLoadBalancer(policy string) LoadBalancer {
	switch policy {
	case "roundrobin":
		return NewRoundRobinLoadBalancer()
	default:
		return NewRandomLoadBalancer()
	}
}

type RandomLoadBalancer struct {
	name string
}

func NewRandomLoadBalancer() *RandomLoadBalancer {
	return &RandomLoadBalancer{
		name: "Random LoadBalancer",
	}
}

func (*RandomLoadBalancer) Pick(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	return backends[rand.IntN(len(backends))]
}

type RoundRobinLoadBalancer struct {
	n atomic.Uint64
}

func NewRoundRobinLoadBalancer() *RoundRobinLoadBalancer {
	return &RoundRobinLoadBalancer{}
}

func (rr *RoundRobinLoadBalancer) Pick(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	i := rr.n.Add(1) - 1
	return backends[i%uint64(len(backends))]
}
