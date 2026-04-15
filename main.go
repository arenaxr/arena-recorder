package main

import (
	"log"
	"os"

	"github.com/arenaxr/arena-recorder/api"
	"github.com/arenaxr/arena-recorder/mqtt"
)

func main() {
	// CLI subcommand: repair
	if len(os.Args) >= 2 && os.Args[1] == "repair" {
		dir := "/recording-store"
		if len(os.Args) >= 3 {
			// repair a single file
			path := os.Args[2]
			n, err := mqtt.RepairIndex(path)
			if err != nil {
				log.Fatalf("Repair failed: %v", err)
			}
			log.Printf("Done — %d keyframes indexed in %s", n, path)
			return
		}
		// repair all files in the store
		if err := mqtt.RepairAllRecordings(dir); err != nil {
			log.Fatalf("Repair failed: %v", err)
		}
		return
	}

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
