package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/zakirov-net/arduino-telemetry/internal/config"
	"github.com/zakirov-net/arduino-telemetry/internal/mqttclient"
	"github.com/zakirov-net/arduino-telemetry/internal/storage"
)

type apiHandler struct {
	store *storage.Store
	mqtt  mqtt.Client
	cfg   config.Web
}

var validBuckets = map[string]string{
	"1m":  "1 minute",
	"5m":  "5 minutes",
	"15m": "15 minutes",
	"1h":  "1 hour",
	"6h":  "6 hours",
	"1d":  "1 day",
}

func (h *apiHandler) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/devices", h.devices)
	mux.HandleFunc("GET /api/readings", h.readings)
	mux.HandleFunc("POST /api/command", h.command)
}

func (h *apiHandler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *apiHandler) devices(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.ListDevices(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *apiHandler) readings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	deviceID64, err := strconv.ParseInt(q.Get("device_id"), 10, 16)
	if err != nil {
		http.Error(w, "device_id must be int16", http.StatusBadRequest)
		return
	}
	from, err := time.Parse(time.RFC3339, q.Get("from"))
	if err != nil {
		http.Error(w, "from must be RFC3339", http.StatusBadRequest)
		return
	}
	to, err := time.Parse(time.RFC3339, q.Get("to"))
	if err != nil {
		http.Error(w, "to must be RFC3339", http.StatusBadRequest)
		return
	}

	bucketKey := q.Get("bucket")
	if bucketKey == "" {
		bucketKey = "1m"
	}
	bucketInterval, ok := validBuckets[bucketKey]
	if !ok {
		http.Error(w, "bucket must be one of 1m|5m|15m|1h|6h|1d", http.StatusBadRequest)
		return
	}

	rows, err := h.store.ReadingsRange(r.Context(), int16(deviceID64), from, to, bucketInterval)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

type commandRequest struct {
	DeviceName string `json:"device_name"`
	Command    string `json:"command"`
}

func (h *apiHandler) command(w http.ResponseWriter, r *http.Request) {
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.DeviceName == "" || req.Command == "" {
		http.Error(w, "device_name and command required", http.StatusBadRequest)
		return
	}
	topic := h.cfg.BaseTopic + "/" + req.DeviceName + "/command"
	if err := mqttclient.Publish(h.mqtt, topic, []byte(req.Command), 1, false); err != nil {
		log.Printf("publish command: %v", err)
		http.Error(w, "mqtt publish failed", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// spaHandler отдаёт файл, если он есть, иначе index.html — чтобы /  и /chart работали.
func spaHandler(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := fsys.Open(r.URL.Path)
		if err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
