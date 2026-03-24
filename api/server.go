package api

import (
	"encoding/json"
	"net/http"

	"github.com/arenaxr/arena-recorder/auth"
)

func StartServer(addr string) error {
	http.HandleFunc("/recorder/start", startRecordingHandler)
	http.HandleFunc("/recorder/stop", stopRecordingHandler)
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
	// Required to initiate a recording
	topic := "realm/s/" + req.Namespace + "/" + req.SceneId + "/#"
	if !auth.HasPublRight(claims, topic) {
		http.Error(w, "Forbidden - Need publish rights to scene", http.StatusForbidden)
		return
	}

	// Trigger MQTT recording logic
	// mqtt.StartRecording(req.Namespace, req.SceneId)

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
	if !auth.HasPublRight(claims, topic) {
		http.Error(w, "Forbidden - Need publish rights to scene", http.StatusForbidden)
		return
	}

	// Trigger MQTT stop logic
	// mqtt.StopRecording(req.Namespace, req.SceneId)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "recording_stopped"})
}
