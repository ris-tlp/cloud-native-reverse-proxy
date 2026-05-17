package main

import (
	"log"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"
	"cloud-native-reverse-proxy/pkg/router"
	"cloud-native-reverse-proxy/pkg/server"
)

func main() {
	reg := registry.NewRegistry()
	r := router.New(reg)
	srv := server.New(":8080", r)

	prov, err := provider.NewDockerProvider()
	if err != nil {
		log.Fatalf("failed to create provider: %v", err)
	}

	go func() {
		if err := prov.Watch(reg); err != nil {
			log.Printf("provider error: %v", err)
		}
	}()

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
