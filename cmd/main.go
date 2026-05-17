package main

import (
	"log"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"
	"cloud-native-reverse-proxy/pkg/router"
	"cloud-native-reverse-proxy/pkg/server"
)

func main() {
	reg := registry.New()
	r := router.New(reg)
	srv := server.New(":8080", r)

	prov := provider.NewDockerProvider()

	go func() {
		if err := prov.Watch(reg); err != nil {
			log.Printf("provider error: %v", err)
		}
	}()

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
