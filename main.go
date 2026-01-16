package main

import (
	"api_gateway/internal/proxy"
	"api_gateway/internal/router"
	"api_gateway/internal/server"
	"fmt"
)

func main() {
	s := server.GetServer()

	mockContainers := [2]router.DockerLabels{
		{"localhost:8001", "/home"},
		{"localhost:8002", "/home_two"},
	}

	routes := []router.Route{
		{
			Host:       "localhost",
			PathPrefix: "/home",
			Proxy:      proxy.GetProxy("http://localhost:8001"),
		},
		{
			Host:       "localhost",
			PathPrefix: "/home_two",
			Proxy:      proxy.GetProxy("http://localhost:8002"),
		},
	}

	for _, route := range mockContainers {
		fmt.Println(route.Host + route.PathPrefix)
	}

	rt := router.NewRouter(routes)
	s.Handler = rt

	err := s.ListenAndServe()
	if err != nil {
		return
	}

}
