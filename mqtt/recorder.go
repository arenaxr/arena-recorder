package mqtt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var client mqtt.Client

// Config holds the service token configuration parsed from /app/config.json
type Config struct {
	JwtServiceToken string `json:"jwt_service_token"`
	JwtServiceUser  string `json:"jwt_service_user"`
}

// serviceConfig is parsed once during Init() and reused by captureInitialState
var serviceConfig *Config

func Init() error {
	opts := mqtt.NewClientOptions()
	brokerUrl := os.Getenv("MQTT_BROKER")
	if brokerUrl == "" {
		brokerUrl = "tcp://mqtt:1883"
	}
	opts.AddBroker(brokerUrl)
	opts.SetClientID("arena-recorder-service")

	// Enable auto-reconnect with handlers to recover active recording sessions
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		log.Printf("Warning: MQTT connection lost: %v. Auto-reconnect is enabled.", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("MQTT connected/reconnected. Re-subscribing active recording sessions...")
		resubscribeActiveSessions(c)
	})

	// Read service token from config.json once and cache it
	tokenFile := "/app/config.json"
	data, err := os.ReadFile(tokenFile)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.JwtServiceToken != "" {
			serviceConfig = &cfg
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

// resubscribeActiveSessions re-subscribes all active recording sessions after a reconnect.
func resubscribeActiveSessions(c mqtt.Client) {
	mu.Lock()
	defer mu.Unlock()

	for key, session := range sessions {
		handler := makeMessageHandler(session)
		if token := c.Subscribe(session.Topic, 0, handler); token.Wait() && token.Error() != nil {
			log.Printf("Error: Failed to re-subscribe session %s: %v", key, token.Error())
		} else {
			log.Printf("Re-subscribed session %s to %s", key, session.Topic)
		}
	}
}

// Recording tracking
var (
	sessions = make(map[string]*RecordingSession)
	mu       sync.Mutex
)

// IndexEntry tracks the byte offset and length for a specific keyframe in the jsonl stream.
// The player can fetch exactly one keyframe via HTTP Range: bytes=Offset-(Offset+Length-1).
type IndexEntry struct {
	Timestamp string `json:"timestamp"`
	Offset    int64  `json:"offset"`
	Length    int64  `json:"length"`
}

// RecordingSession tracks a single active recording to a scene
type RecordingSession struct {
	Namespace         string
	SceneId           string
	Topic             string
	File              *os.File
	Writer            *bufio.Writer
	writeMu           sync.Mutex // serializes concurrent MQTT callback writes
	bytesWritten      int64
	lastKeyframeBytes int64
	compactedState    map[string]map[string]interface{}
	index             []IndexEntry
}

// deepMerge recursively merges two maps representing JSON objects.
// Arrays and primitive values are overwritten.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = make(map[string]interface{})
	}
	for k, v := range src {
		if srcMap, okSrc := v.(map[string]interface{}); okSrc {
			if dstMap, okDst := dst[k].(map[string]interface{}); okDst {
				dst[k] = deepMerge(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

// writeLine safely writes a line to the session's buffered writer, maintains compactedState,
// and ensures we emit keyframes every 2MB of deltas.
func (s *RecordingSession) writeLine(timestamp string, payload map[string]interface{}, line string) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Maintain state
	if payload != nil {
		if objIdVal, ok := payload["object_id"]; ok {
			if objId, okStr := objIdVal.(string); okStr {
				actionVal, _ := payload["action"]
				action, _ := actionVal.(string)
				if action == "delete" {
					delete(s.compactedState, objId)
				} else if action == "create" || action == "update" {
					s.compactedState[objId] = deepMerge(s.compactedState[objId], payload)
				}
			}
		}
	}

	n, _ := s.Writer.WriteString(line)
	s.bytesWritten += int64(n)
	m, _ := s.Writer.WriteString("\n")
	s.bytesWritten += int64(m)

	if s.bytesWritten-s.lastKeyframeBytes >= 1024*1024 { // 1MB of deltas between keyframes
		s.emitKeyframeLocked(timestamp)
	}
}

// emitKeyframeLocked dumps the internal compactedState directly to the buffered writer.
// Caller must hold writeMu.
func (s *RecordingSession) emitKeyframeLocked(timestamp string) {
	kf := map[string]interface{}{
		"action":    "keyframe",
		"timestamp": timestamp,
		"state":     s.compactedState,
	}
	b, err := json.Marshal(kf)
	if err != nil {
		return
	}
	line := string(b)
	kfOffset := s.bytesWritten
	kfLength := int64(len(line))

	n, _ := s.Writer.WriteString(line)
	s.bytesWritten += int64(n)
	m, _ := s.Writer.WriteString("\n")
	s.bytesWritten += int64(m)
	s.lastKeyframeBytes = s.bytesWritten

	s.index = append(s.index, IndexEntry{
		Timestamp: timestamp,
		Offset:    kfOffset,
		Length:    kfLength,
	})
}

// flushAndClose flushes the buffered writer and closes the underlying file
func (s *RecordingSession) flushAndClose() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.Writer.Flush(); err != nil {
		s.File.Close()
		return fmt.Errorf("flush error: %v", err)
	}
	if err := s.File.Sync(); err != nil {
		s.File.Close()
		return fmt.Errorf("sync error: %v", err)
	}
	return s.File.Close()
}

// publishChatCtrl sends a chat control plane message for recording state changes
func publishChatCtrl(namespace, sceneId, text string) {
	chatTopicArgs := map[string]string{
		"nameSpace":  namespace,
		"sceneName":  sceneId,
		"userClient": "arena-recorder",
		"idTag":      "recorder",
	}
	chatTopic := FormatTopic(Topics.Publish.SceneChat, chatTopicArgs)
	chatMsg := []byte(fmt.Sprintf(`{"object_id": "recorder", "action": "recording", "type": "chat-ctrl", "text": "%s", "dn": "Recorder"}`, text))
	client.Publish(chatTopic, 1, true, chatMsg)
}

// makeMessageHandler creates the MQTT message handler closure for a recording session.
// This is extracted so it can be reused during reconnection re-subscription.
func makeMessageHandler(session *RecordingSession) mqtt.MessageHandler {
	return func(c mqtt.Client, msg mqtt.Message) {
		var payload map[string]interface{}
		ts := time.Now().Format(time.RFC3339Nano)
		if err := json.Unmarshal(msg.Payload(), &payload); err == nil {
			// Inject server receipt timestamp to guarantee monotonically increasing time
			payload["timestamp"] = ts
			if b, err := json.Marshal(payload); err == nil {
				session.writeLine(ts, payload, string(b))
				return
			}
		}
		// Fallback if parsing fails — write raw payload
		session.writeLine(ts, nil, string(msg.Payload()))
	}
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
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("could not create recording file: %v", err)
	}

	writer := bufio.NewWriterSize(file, 64*1024) // 64KB write buffer

	topicArgs := map[string]string{
		"nameSpace": namespace,
		"sceneName": sceneId,
	}
	sceneTopic := FormatTopic(Topics.Subscribe.ScenePublic, topicArgs)
	// Replace the wildcards to subscribe to everything in the scene for recording
	sceneTopic = strings.ReplaceAll(sceneTopic, "+/+/+", "#")

	session := &RecordingSession{
		Namespace:         namespace,
		SceneId:           sceneId,
		Topic:             sceneTopic,
		File:              file,
		Writer:            writer,
		compactedState:    make(map[string]map[string]interface{}),
		index:             make([]IndexEntry, 0),
	}

	// Fetch initial state from arena-persist
	persistURL := fmt.Sprintf("http://arena-persist:8884/persist/%s/%s", namespace, sceneId)
	if err := captureInitialState(persistURL, session); err != nil {
		log.Printf("Warning: Failed to capture initial state: %v", err)
		// We still continue to record live events
	} else {
		// Emit initial keyframe point immediately after reading starting initial state
		session.writeMu.Lock()
		session.emitKeyframeLocked(time.Now().Format(time.RFC3339Nano))
		session.writeMu.Unlock()
	}

	// Subscribe to the scene topic BEFORE publishing chat-ctrl
	handler := makeMessageHandler(session)
	if token := client.Subscribe(session.Topic, 0, handler); token.Wait() && token.Error() != nil {
		session.flushAndClose()
		// Publish recording_failed so arena-web-core can display it
		publishChatCtrl(namespace, sceneId, "recording_failed")
		return fmt.Errorf("failed to subscribe: %v", token.Error())
	}

	// Publish recording_started banner only after successful subscription
	publishChatCtrl(namespace, sceneId, "recording_started")

	sessions[key] = session
	log.Printf("Started recording %s to %s", key, filename)
	return nil
}

func captureInitialState(persistURL string, session *RecordingSession) error {
	// Call persist to get initial objects
	req, err := http.NewRequest("GET", persistURL, nil)
	if err != nil {
		return err
	}

	// Use the cached service config for auth
	if serviceConfig != nil && serviceConfig.JwtServiceToken != "" {
		req.AddCookie(&http.Cookie{Name: "mqtt_token", Value: serviceConfig.JwtServiceToken})
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
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
			session.writeLine(now, obj, string(b))
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

	// Unsubscribe and check for errors
	if token := client.Unsubscribe(session.Topic); token.Wait() && token.Error() != nil {
		log.Printf("Warning: Failed to unsubscribe %s: %v", session.Topic, token.Error())
	}

	// Write the metadata index line at the end before closing
	session.writeMu.Lock()
	indexLine := map[string]interface{}{
		"action": "keyframe_index",
		"index":  session.index,
	}
	if b, err := json.Marshal(indexLine); err == nil {
		line := string(b)
		session.Writer.WriteString(line)
		session.Writer.WriteString("\n")
	}
	session.writeMu.Unlock()

	// Flush buffered writes, sync to disk, and close
	if err := session.flushAndClose(); err != nil {
		log.Printf("Warning: Error closing recording file for %s: %v", key, err)
	}
	delete(sessions, key)

	// Publish Recording banner stop via Chat Control Plane
	publishChatCtrl(namespace, sceneId, "recording_stopped")

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
