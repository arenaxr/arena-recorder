package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

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

// Recording tracking
var (
	sessions = make(map[string]*RecordingSession)
	mu       sync.Mutex
)

type RecordingSession struct {
	Namespace string
	SceneId   string
	Topic     string
	File      *os.File
}

func StartRecording(namespace, sceneId string) error {
	mu.Lock()
	defer mu.Unlock()

	key := namespace + "/" + sceneId
	if _, exists := sessions[key]; exists {
		return fmt.Errorf("already recording %s", key)
	}

	// Make sure directory exists
	os.MkdirAll("/recording-store", 0755)

	filename := fmt.Sprintf("/recording-store/%s~%s~%d.jsonl", namespace, sceneId, time.Now().Unix())
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("could not create recording file: %v", err)
	}

	topicArgs := map[string]string{
		"nameSpace": namespace,
		"sceneName": sceneId,
	}
	sceneTopic := FormatTopic(Topics.Subscribe.ScenePublic, topicArgs)
	// Replace the wildcards to subscribe to everything in the scene for recording
	sceneTopic = strings.ReplaceAll(sceneTopic, "+/+/+", "#")

	session := &RecordingSession{
		Namespace: namespace,
		SceneId:   sceneId,
		Topic:     sceneTopic,
		File:      file,
	}

	// Fetch initial state from arena-persist
	persistURL := fmt.Sprintf("http://arena-persist:8884/persist/%s/%s", namespace, sceneId)
	if err := captureInitialState(persistURL, session.File); err != nil {
		log.Printf("Warning: Failed to capture initial state: %v", err)
		// We still continue to record live events
	}

	// Publish Recording banner via Chat Control Plane
	chatTopicArgs := map[string]string{
		"nameSpace":  namespace,
		"sceneName":  sceneId,
		"userClient": "arena-recorder",
		"idTag":      "recorder",
	}
	chatTopic := FormatTopic(Topics.Publish.SceneChat, chatTopicArgs)
	chatMsg := []byte(`{"object_id": "recorder", "action": "recording", "type": "chat-ctrl", "text": "recording_started", "dn": "Recorder"}`)
	client.Publish(chatTopic, 1, true, chatMsg)

	// Message handler
	handler := func(client mqtt.Client, msg mqtt.Message) {
		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Payload(), &payload); err == nil {
			// Inject server receipt timestamp to guarantee monotonically increasing time
			payload["timestamp"] = time.Now().Format(time.RFC3339Nano)
			if b, err := json.Marshal(payload); err == nil {
				session.File.WriteString(string(b) + "\n")
				return
			}
		}
		// Fallback if parsing fails
		line := string(msg.Payload()) + "\n"
		session.File.WriteString(line)
	}

	if token := client.Subscribe(session.Topic, 0, handler); token.Wait() && token.Error() != nil {
		session.File.Close()
		return fmt.Errorf("failed to subscribe: %v", token.Error())
	}

	sessions[key] = session
	log.Printf("Started recording %s to %s", key, filename)
	return nil
}

func captureInitialState(persistURL string, file *os.File) error {
	// Call persist to get initial objects
	req, err := http.NewRequest("GET", persistURL, nil)
	if err != nil {
		return err
	}

	// We should include the jwt_service_token if it exists, since persist requires auth for private scenes
	tokenFile := "/app/config.json"
	data, err := os.ReadFile(tokenFile)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.JwtServiceToken != "" {
			req.AddCookie(&http.Cookie{Name: "mqtt_token", Value: cfg.JwtServiceToken})
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("persist returned status %d", resp.StatusCode)
	}

	var objects []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&objects); err != nil {
		return err
	}

	// Write each object as an 'action: create' message
	now := time.Now().Format(time.RFC3339Nano)
	for _, obj := range objects {
		obj["action"] = "create"
		obj["timestamp"] = now
		
		// Map 'attributes' from MongoDB schema to 'data' for MQTT wire protocol schema
		if attr, ok := obj["attributes"]; ok {
			obj["data"] = attr
			delete(obj, "attributes")
		}
		
		b, err := json.Marshal(obj)
		if err == nil {
			file.WriteString(string(b) + "\n")
		}
	}

	return nil
}

func StopRecording(namespace, sceneId string) error {
	mu.Lock()
	defer mu.Unlock()

	key := namespace + "/" + sceneId
	session, exists := sessions[key]
	if !exists {
		return fmt.Errorf("not recording %s", key)
	}

	client.Unsubscribe(session.Topic).Wait()
	session.File.Close()
	delete(sessions, key)

	// Publish Recording banner stop via Chat Control Plane
	topicArgs := map[string]string{
		"nameSpace":  namespace,
		"sceneName":  sceneId,
		"userClient": "arena-recorder",
		"idTag":      "recorder",
	}
	chatTopic := FormatTopic(Topics.Publish.SceneChat, topicArgs)
	chatMsg := []byte(`{"object_id": "recorder", "action": "recording", "type": "chat-ctrl", "text": "recording_stopped", "dn": "Recorder"}`)
	client.Publish(chatTopic, 1, true, chatMsg)

	log.Printf("Stopped recording %s", key)
	return nil
}

func IsRecording(namespace, sceneId string) bool {
	mu.Lock()
	defer mu.Unlock()

	key := namespace + "/" + sceneId
	_, exists := sessions[key]
	return exists
}
