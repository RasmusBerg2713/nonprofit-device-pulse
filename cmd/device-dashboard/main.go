package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/infrai-examples/nonprofit-device-pulse/internal/dashboard"
	"github.com/infrai-examples/nonprofit-device-pulse/internal/infrai"
)

type publisher interface {
	ReportDeviceSeen(context.Context, string, string) error
	QueryCampaignMetrics(context.Context, string) (json.RawMessage, error)
	Publish(context.Context, string, string, string, any) error
}

type statusHandler struct {
	infrai  publisher
	channel string
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	baseURL := envOr("INFRAI_BASE_URL", infrai.DefaultBaseURL)
	channel := envOr("INFRAI_CHANNEL", "nonprofit-device-status")
	address := envOr("LISTEN_ADDR", ":8080")

	client := infrai.New(baseURL, key, nil)
	handler := statusHandler{infrai: client, channel: channel}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /device-status", handler.serveHTTP)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("device dashboard relay listening on %s", address)
	log.Fatal(server.ListenAndServe())
}

func (h statusHandler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	var status dashboard.DeviceStatus
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device status"})
		return
	}
	if err := validate(status); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	id := eventID(status)
	if err := h.infrai.ReportDeviceSeen(r.Context(), status.CampaignID, id+"-metric"); err != nil {
		writeUpstreamError(w, err)
		return
	}
	metrics, err := h.infrai.QueryCampaignMetrics(r.Context(), status.CampaignID)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	update := dashboard.Decide(status, metrics)
	if err := h.infrai.Publish(r.Context(), h.channel, status.CampaignID, id+"-publish", update); err != nil {
		writeUpstreamError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"event_id": id, "update": update})
}

func validate(status dashboard.DeviceStatus) error {
	if strings.TrimSpace(status.DeviceID) == "" || strings.TrimSpace(status.CampaignID) == "" || strings.TrimSpace(status.DonorID) == "" || strings.TrimSpace(status.VolunteerID) == "" {
		return errors.New("device_id, campaign_id, donor_id, and volunteer_id are required")
	}
	if status.BatteryPercent < 0 || status.BatteryPercent > 100 {
		return errors.New("battery_percent must be between 0 and 100")
	}
	if status.ObservedAt.IsZero() {
		return errors.New("observed_at is required")
	}
	return nil
}

func eventID(status dashboard.DeviceStatus) string {
	sum := sha256.Sum256([]byte(status.DeviceID + "\x00" + status.ObservedAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:12])
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	var apiErr *infrai.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
		return
	}
	log.Printf("Infrai request failed: %v", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "dashboard update was not accepted"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
