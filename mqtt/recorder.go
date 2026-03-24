package mqtt

import (
	"encoding/json"
	"log"
	"os"

	"github.com/eclipse/paho.mqtt.golang"
)

var client mqtt.Client

type Config struct {
	JwtServiceToken string `json:"jwt_service_token"`
	JwtServiceUser  string `json:"jwt_service_user"`
}

func Init() error {
	opts := mqtt.NewClientOptions()
	brokerUrl := os.Getenv("MQTT_BROKER")
	if brokerUrl == "" {
		brokerUrl = "tcp://mqtt:1883"
	}
	opts.AddBroker(brokerUrl)
	opts.SetClientID("arena-recorder-service")

	// Read service token from config.json if available
	tokenFile := "/app/config.json"
	data, err := os.ReadFile(tokenFile)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.JwtServiceToken != "" {
			if cfg.JwtServiceUser != "" {
				opts.SetUsername(cfg.JwtServiceUser)
			} else {
				opts.SetUsername("arena-recorder")
			}
			opts.SetPassword(cfg.JwtServiceToken)
			log.Println("Loaded MQTT service token from config.json")
		}
	} else {
		log.Println("Warning: Could not read /app/config.json for MQTT token")
	}

	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	log.Println("Connected to MQTT broker at", brokerUrl)
	return nil
}

// In a full implementation, StartRecording would:
// 1. Subscribe to realm/s/<namespace>/<sceneId>/#
// 2. Open a new .jsonl file in /recording-store/<uuid>.jsonl
// 3. Set up a messageHandler to marshal payloads and flush to disk
// 4. Track active recordings in a map
