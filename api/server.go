package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/arenaxr/arena-recorder/auth"
	"github.com/arenaxr/arena-recorder/mqtt"
)

func StartServer(addr string) error {
	http.HandleFunc("/recorder/start", startRecordingHandler)
	http.HandleFunc("/recorder/stop", stopRecordingHandler)
	http.HandleFunc("/recorder/list", listRecordingsHandler)
	http.HandleFunc("/recorder/status", recordingStatusHandler)
	http.HandleFunc("/recorder/files/", serveRecordingFileHandler)
	log.Printf("Starting REST API server on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func startRecordingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, err := auth.ValidateMQTTToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Namespace string `json:"namespace"`
		SceneId   string `json:"sceneId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Verify they have publish rights to realm/s/<namespace>/<sceneId>/#
	// OR if they are the owner of the namespace OR have any object publish rights
	topicArgs := map[string]string{
		"nameSpace": req.Namespace,
		"sceneName": req.SceneId,
	}
	topic := mqtt.FormatTopic(mqtt.Topics.Subscribe.ScenePublic, topicArgs)
	topic = strings.ReplaceAll(topic, "+/+/+", "#")
	if claims.Subject != req.Namespace && !auth.HasPublRight(claims, topic) && !auth.CanRecordScene(claims, req.Namespace, req.SceneId) {
		http.Error(w, "Forbidden - Need publish rights to scene or namespace ownership", http.StatusForbidden)
		return
	}

	// Trigger MQTT recording logic
	if err := mqtt.StartRecording(req.Namespace, req.SceneId); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "recording_started"})
}

func stopRecordingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, err := auth.ValidateMQTTToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Namespace string `json:"namespace"`
		SceneId   string `json:"sceneId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	topicArgs := map[string]string{
		"nameSpace": req.Namespace,
		"sceneName": req.SceneId,
	}
	topic := mqtt.FormatTopic(mqtt.Topics.Subscribe.ScenePublic, topicArgs)
	topic = strings.ReplaceAll(topic, "+/+/+", "#")
	if claims.Subject != req.Namespace && !auth.HasPublRight(claims, topic) && !auth.CanRecordScene(claims, req.Namespace, req.SceneId) {
		http.Error(w, "Forbidden - Need publish rights to scene or namespace ownership", http.StatusForbidden)
		return
	}

	// Trigger MQTT stop logic
	if err := mqtt.StopRecording(req.Namespace, req.SceneId); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "recording_stopped"})
}

func listRecordingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, err := auth.ValidateMQTTToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	files, err := os.ReadDir("/recording-store")
	if err != nil {
		http.Error(w, "Failed to read recordings", http.StatusInternalServerError)
		return
	}

	// Initialize as empty slice so JSON encodes as [] instead of null
	recordings := make([]map[string]string, 0)
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
			name := strings.TrimSuffix(f.Name(), ".jsonl")

			var namespace, sceneId, timestamp string
			parts := strings.Split(name, "~")
			if len(parts) == 3 {
				namespace = parts[0]
				sceneId = parts[1]
				timestamp = parts[2]
			}

			if namespace != "" && sceneId != "" {
				record := map[string]string{"filename": f.Name()}
				topicArgs := map[string]string{
					"nameSpace": namespace,
					"sceneName": sceneId,
				}
				topic := mqtt.FormatTopic(mqtt.Topics.Subscribe.ScenePublic, topicArgs)
				topic = strings.ReplaceAll(topic, "+/+/+", "#")

				// Filter list by subl rights (allow owner, public namespace, or explicit JWT claims)
				if claims.Subject != namespace && namespace != "public" && !auth.HasSubRight(claims, topic) && !auth.HasPublRight(claims, topic) {
					continue
				}

				record["timestamp"] = timestamp
				record["name"] = name
				recordings = append(recordings, record)
			} else {
				recordings = append(recordings, map[string]string{"filename": f.Name(), "name": name})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recordings)
}

func recordingStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_, err := auth.ValidateMQTTToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	sceneId := r.URL.Query().Get("sceneId")
	if namespace == "" || sceneId == "" {
		http.Error(w, "Bad request: missing namespace or sceneId", http.StatusBadRequest)
		return
	}

	isRecording := mqtt.IsRecording(namespace, sceneId)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"is_recording": isRecording})
}

func serveRecordingFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, err := auth.ValidateMQTTToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/recorder/files/")

	// Strict path sanitization: only allow flat filenames (no directory traversal)
	cleanName := filepath.Base(filename)
	if cleanName != filename || filename == "" || !strings.HasSuffix(filename, ".jsonl") {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSuffix(filename, ".jsonl")

	var namespace, sceneId string
	parts := strings.Split(name, "~")
	if len(parts) == 3 {
		namespace = parts[0]
		sceneId = parts[1]
	}

	if namespace == "" || sceneId == "" {
		http.Error(w, "Invalid filename format", http.StatusBadRequest)
		return
	}

	// Enforce ACL: user must have subscribe or publish rights to the scene
	// NOTE: arena-web-core /replay page should re-request the JWT token (with filled in
	// namespace/sceneId params in the user/mqtt_auth request) before each namespace/sceneId
	// change to ensure the JWT has the required subs rights for the specific scene.
	topicArgs := map[string]string{
		"nameSpace": namespace,
		"sceneName": sceneId,
	}
	topic := mqtt.FormatTopic(mqtt.Topics.Subscribe.ScenePublic, topicArgs)
	topic = strings.ReplaceAll(topic, "+/+/+", "#")
	if claims.Subject != namespace && namespace != "public" &&
		!auth.HasSubRight(claims, topic) && !auth.HasPublRight(claims, topic) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, "/recording-store/"+filename)
}
