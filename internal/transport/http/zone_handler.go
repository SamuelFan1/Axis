package http

import (
	"encoding/json"
	"net/http"

	"github.com/SamuelFan1/Axis/internal/service"
)

type ZoneHandler struct {
	zoneService *service.ZoneService
}

func NewZoneHandler(zoneService *service.ZoneService) *ZoneHandler {
	return &ZoneHandler{zoneService: zoneService}
}

type createZoneRequest struct {
	Name string `json:"name"`
}

func (h *ZoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid json body"})
		return
	}
	z, err := h.zoneService.Create(r.Context(), req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "zone created",
		"zone":    map[string]string{"uuid": z.UUID, "name": z.Name},
	})
}

func (h *ZoneHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.zoneService.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"zones": items,
		"count": len(items),
	})
}

func (h *ZoneHandler) Delete(w http.ResponseWriter, r *http.Request, uuid string) {
	if err := h.zoneService.DeleteByUUID(r.Context(), uuid); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "zone deleted",
		"uuid":    uuid,
	})
}
