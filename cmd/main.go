package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"cloud-native-reverse-proxy/pkg/provider"
	"cloud-native-reverse-proxy/pkg/registry"
	"cloud-native-reverse-proxy/pkg/router"
	"cloud-native-reverse-proxy/pkg/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := registry.NewRegistry()
	r := router.New(reg)
	srv := server.New(":8080", r)

	prov, err := provider.NewDockerProvider()
	if err != nil {
		log.Fatalf("failed to create provider: %v", err)
	}

	go func() {
		if err := prov.Watch(ctx, reg); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("provider error: %v", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		log.Println("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			log.Printf("server error: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
