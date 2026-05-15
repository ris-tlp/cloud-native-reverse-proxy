// Package provider
package provider

import "cloud-native-reverse-proxy/pkg/registry"

type Provider interface {
	Watch(reg *registry.Registry)
}
