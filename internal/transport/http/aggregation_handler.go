package http

import (
	"encoding/json"
	"net/http"

	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type AggregationHandler struct {
	snapshotRepo repository.RegionalNodeStatusSnapshotRepository
}

func NewAggregationHandler(snapshotRepo repository.RegionalNodeStatusSnapshotRepository) *AggregationHandler {
	return &AggregationHandler{snapshotRepo: snapshotRepo}
}

func (h *AggregationHandler) IngestRegionalSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return
	}
	if h.snapshotRepo == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "aggregation receiver is disabled"})
		return
	}

	var snapshot node.RegionalNodeStatusSnapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json body"})
		return
	}
	if err := h.snapshotRepo.UpsertSnapshot(r.Context(), snapshot); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "regional snapshot ingested",
		"region":  snapshot.SourceRegion,
		"count":   len(snapshot.Nodes),
	})
}
