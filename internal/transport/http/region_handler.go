package http

import (
	"encoding/json"
	"net/http"

	"github.com/SamuelFan1/Axis/internal/service"
)

type RegionHandler struct {
	regionService *service.RegionService
}

func NewRegionHandler(regionService *service.RegionService) *RegionHandler {
	return &RegionHandler{regionService: regionService}
}

type createRegionRequest struct {
	Name string `json:"name"`
}

func (h *RegionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRegionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json body"})
		return
	}
	reg, err := h.regionService.Create(r.Context(), req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "region created",
		"region":  map[string]string{"uuid": reg.UUID, "name": reg.Name},
	})
}

func (h *RegionHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.regionService.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"regions": items,
		"count":   len(items),
	})
}

func (h *RegionHandler) Delete(w http.ResponseWriter, r *http.Request, uuid string) {
	if err := h.regionService.DeleteByUUID(r.Context(), uuid); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "region deleted",
		"uuid":    uuid,
	})
}
