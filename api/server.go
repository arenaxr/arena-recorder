package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/arenaxr/arena-recorder/auth"
	"github.com/arenaxr/arena-recorder/mqtt"
)

func StartServer(addr string) error {
	http.HandleFunc("/recorder/start", startRecordingHandler)
	http.HandleFunc("/recorder/stop", stopRecordingHandler)
	http.HandleFunc("/recorder/list", listRecordingsHandler)
	http.HandleFunc("/recorder/files/", serveRecordingFileHandler)
	return http.ListenAndServe(addr, nil)
}

func startRecordingHandler(w http.ResponseWriter, r *http.Request) {
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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "recording_started"})
}

func stopRecordingHandler(w http.ResponseWriter, r *http.Request) {
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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "recording_stopped"})
}

func listRecordingsHandler(w http.ResponseWriter, r *http.Request) {
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

	var recordings []map[string]string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
			name := strings.TrimSuffix(f.Name(), ".jsonl")
			parts := strings.Split(name, "-")
			record := map[string]string{"filename": f.Name()}
			if len(parts) >= 3 {
				timestamp := parts[len(parts)-1]
				sceneId := parts[len(parts)-2]
				namespace := strings.Join(parts[:len(parts)-2], "-")
				
				topicArgs := map[string]string{
					"nameSpace": namespace,
					"sceneName": sceneId,
				}
				topic := mqtt.FormatTopic(mqtt.Topics.Subscribe.ScenePublic, topicArgs)
				topic = strings.ReplaceAll(topic, "+/+/+", "#")

				// Filter list by subl rights
				if claims.Subject != namespace && !auth.HasSubRight(claims, topic) && !auth.HasPublRight(claims, topic) {
					continue
				}

				record["timestamp"] = timestamp
				record["name"] = name
			} else {
				record["name"] = name
			}
			recordings = append(recordings, record)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recordings)
}

func serveRecordingFileHandler(w http.ResponseWriter, r *http.Request) {
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
	if filename == "" || strings.Contains(filename, "..") || !strings.HasSuffix(filename, ".jsonl") {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSuffix(filename, ".jsonl")
	parts := strings.Split(name, "-")
	if len(parts) >= 3 {
		sceneId := parts[len(parts)-2]
		namespace := strings.Join(parts[:len(parts)-2], "-")
		
		topicArgs := map[string]string{
			"nameSpace": namespace,
			"sceneName": sceneId,
		}
		topic := mqtt.FormatTopic(mqtt.Topics.Subscribe.ScenePublic, topicArgs)
		topic = strings.ReplaceAll(topic, "+/+/+", "#")

		if claims.Subject != namespace && !auth.HasSubRight(claims, topic) && !auth.HasPublRight(claims, topic) {
			http.Error(w, "Forbidden - Need subscribe rights to scene", http.StatusForbidden)
			return
		}
	} else {
		http.Error(w, "Invalid filename format", http.StatusBadRequest)
		return
	}

	http.ServeFile(w, r, "/recording-store/"+filename)
}
