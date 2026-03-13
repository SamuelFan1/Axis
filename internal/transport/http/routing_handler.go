package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/observation"
	"github.com/SamuelFan1/Axis/internal/service"
)

type RoutingHandler struct {
	observationService *service.RoutingObservationService
	snapshotService    *service.RoutingSnapshotService
	publishService     *service.RoutingPublishService
}

type recordObservationsRequest struct {
	Observations []observation.RecordInput `json:"observations"`
}

func NewRoutingHandler(
	observationService *service.RoutingObservationService,
	snapshotService *service.RoutingSnapshotService,
	publishService *service.RoutingPublishService,
) *RoutingHandler {
	return &RoutingHandler{
		observationService: observationService,
		snapshotService:    snapshotService,
		publishService:     publishService,
	}
}

func (h *RoutingHandler) RecordObservations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}
	if h.observationService == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "routing observation is disabled"})
		return
	}

	var req recordObservationsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json body"})
		return
	}
	if len(req.Observations) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "observations are required"})
		return
	}

	if err := h.observationService.Record(r.Context(), req.Observations); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "routing observations recorded",
		"count":   len(req.Observations),
	})
}

func (h *RoutingHandler) LatestSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}
	if h.snapshotService == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "routing snapshot is disabled"})
		return
	}

	manifest, bundles, err := h.snapshotService.GetLatest(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	if manifest == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "routing snapshot not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"manifest": manifest,
		"bundles":  bundles,
	})
}

func (h *RoutingHandler) GenerateSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}
	if h.snapshotService == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "routing snapshot is disabled"})
		return
	}

	manifest, bundles, err := h.snapshotService.GenerateAndStore(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	published := false
	if h.publishService != nil && h.publishService.Enabled() {
		if err := h.publishService.Publish(r.Context(), manifest, bundles); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		published = true
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"manifest":  manifest,
		"bundles":   bundles,
		"published": published,
	})
}

func (h *RoutingHandler) SnapshotByVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}
	if h.snapshotService == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "routing snapshot is disabled"})
		return
	}

	version, ok := extractRoutingSnapshotVersion(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid routing snapshot path"})
		return
	}

	manifest, bundles, err := h.snapshotService.GetByVersion(r.Context(), version)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	if manifest == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "routing snapshot not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"manifest": manifest,
		"bundles":  bundles,
	})
}

func extractRoutingSnapshotVersion(path string) (string, bool) {
	trimmed := strings.TrimPrefix(path, "/api/v1/routing/snapshots/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", false
	}
	if trimmed == "latest" || trimmed == "generate" {
		return "", false
	}
	return trimmed, true
}
