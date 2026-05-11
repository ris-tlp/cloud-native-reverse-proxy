package main

import (
	"log"

	"cloud-native-reverse-proxy/pkg/registry"
	"cloud-native-reverse-proxy/pkg/router"
	"cloud-native-reverse-proxy/pkg/server"
)

func main() {
	registry := registry.New()
	router := router.New(registry)
	server := server.New(":8080", router)

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	// ctx := context.Background()
	// apiClient, err := client.New(client.FromEnv, client.WithUserAgent("my-application/1.0.0"))
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer apiClient.Close()
	// result := apiClient.Events(ctx, client.EventsListOptions{})
	//
	// var wg sync.WaitGroup
	//
	// wg.Add(1)
	// go func() {
	// 	defer wg.Done()
	// 	for {
	// 		select {
	// 		case res, ok := <-result.Messages:
	// 			if !ok {
	// 				return
	// 			}
	// 			log.Println(res)
	// 		case error := <-result.Err:
	// 			log.Fatal(error)
	// 		case <-ctx.Done():
	// 			return
	// 		}
	// 	}
	// }()
	//
	// wg.Add(1)
	// go concurrentPrint(&wg)
	//
	// wg.Wait()
}

// func concurrentPrint(wg *sync.WaitGroup) {
// 	defer wg.Done()
// 	fmt.Println("big man big goroutine")
// }
