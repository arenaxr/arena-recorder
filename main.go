package main

import (
	"log"

	"github.com/arenaxr/arena-recorder/api"
	"github.com/arenaxr/arena-recorder/mqtt"
)

func main() {
	log.Println("Starting arena-recorder...")

	// Initialize MQTT client and subscribe
	if err := mqtt.Init(); err != nil {
		log.Fatalf("Failed to initialize MQTT: %v", err)
	}

	// Start API server
	if err := api.StartServer(":8885"); err != nil {
		log.Fatalf("Failed to start API server: %v", err)
	}
}
