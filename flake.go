package main

import (
	"flake/server"
	"log"
)

func main() {

	cfg := server.Config{
		Endpoints:     []string{"http://127.0.0.1:32379"},
		ListenAddress: "127.0.0.1:30001",
		Prefix:        "/flake/",
	}

	_, err := server.StartServer(&cfg)
	if err != nil {
		log.Printf("start server failed. %v", err)
	}
}
