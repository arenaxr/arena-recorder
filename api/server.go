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
	http.Handle("/recorder/files/", http.StripPrefix("/recorder/files/", http.FileServer(http.Dir("/recording-store"))))
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
	topic := "realm/s/" + req.Namespace + "/" + req.SceneId + "/#"
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

	topic := "realm/s/" + req.Namespace + "/" + req.SceneId + "/#"
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

	files, err := os.ReadDir("/recording-store")
	if err != nil {
		http.Error(w, "Failed to read recordings", http.StatusInternalServerError)
		return
	}

	var recordings []map[string]string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
			// Extract namespace, scene and timestamp if possible
			// Format: {namespace}-{sceneId}-{timestamp}.jsonl
			name := strings.TrimSuffix(f.Name(), ".jsonl")
			parts := strings.Split(name, "-")
			record := map[string]string{"filename": f.Name()}
			if len(parts) >= 3 {
				// The last part is timestamp
				record["timestamp"] = parts[len(parts)-1]
				// The rest is namespace and scene. It's ambiguous if they have dashes.
				// For now just pass the whole thing
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
