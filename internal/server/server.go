package server

import (
	"net/http"
)

func GetServer() *http.Server {
	server := &http.Server{
		Addr: ":8080",
	}

	return server
}
