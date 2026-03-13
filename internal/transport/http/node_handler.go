package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/service"
)

type NodeHandler struct {
	nodeService *service.NodeService
}

type registerNodeRequest struct {
	UUID              string `json:"uuid"`
	Hostname          string `json:"hostname"`
	ManagementAddress string `json:"management_address"`
	Region            string `json:"region"`
	Zone              string `json:"zone"`
	Status            string `json:"status"`
}

type updateNodeStatusRequest struct {
	Status string `json:"status"`
}

type reportNodeRequest struct {
	UUID               string            `json:"uuid"`
	Hostname           string            `json:"hostname"`
	ManagementAddress  string            `json:"management_address"`
	InternalIP         string            `json:"internal_ip"`
	PublicIP           string            `json:"public_ip"`
	Region             string            `json:"region"`
	Zone               string            `json:"zone"`
	Status             string            `json:"status"`
	CPUCores           int               `json:"cpu_cores"`
	CPUUsagePercent    float64           `json:"cpu_usage_percent"`
	MemoryTotalGB      float64           `json:"memory_total_gb"`
	MemoryUsedGB       float64           `json:"memory_used_gb"`
	MemoryUsagePercent float64           `json:"memory_usage_percent"`
	SwapTotalGB        float64           `json:"swap_total_gb"`
	SwapUsedGB         float64           `json:"swap_used_gb"`
	SwapUsagePercent   float64           `json:"swap_usage_percent"`
	DiskUsagePercent   float64           `json:"disk_usage_percent"`
	DiskDetails        []node.DiskDetail `json:"disk_details"`
	MonitoringSnapshot json.RawMessage   `json:"monitoring_snapshot,omitempty"`
}

func NewNodeHandler(nodeService *service.NodeService) *NodeHandler {
	return &NodeHandler{nodeService: nodeService}
}

func (h *NodeHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	var req registerNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid json body",
		})
		return
	}

	item := node.Node{
		UUID:              req.UUID,
		Hostname:          req.Hostname,
		ManagementAddress: req.ManagementAddress,
		Region:            req.Region,
		Zone:              req.Zone,
		Status:            req.Status,
	}

	registered, err := h.nodeService.Register(r.Context(), item)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "node registered",
		"node": map[string]string{
			"uuid":               registered.UUID,
			"hostname":           registered.Hostname,
			"management_address": registered.ManagementAddress,
			"region":             registered.Region,
			"zone":               registered.Zone,
			"status":             registered.Status,
		},
	})
}

func (h *NodeHandler) RegisterAdmin(w http.ResponseWriter, r *http.Request) {
	h.Register(w, r)
}

func (h *NodeHandler) Report(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	var req reportNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid json body",
		})
		return
	}

	reported, err := h.nodeService.Report(r.Context(), node.Node{
		UUID:               req.UUID,
		Hostname:           req.Hostname,
		ManagementAddress:  req.ManagementAddress,
		InternalIP:         req.InternalIP,
		PublicIP:           req.PublicIP,
		Region:             req.Region,
		Zone:               req.Zone,
		Status:             req.Status,
		CPUCores:           req.CPUCores,
		CPUUsagePercent:    req.CPUUsagePercent,
		MemoryTotalGB:      req.MemoryTotalGB,
		MemoryUsedGB:       req.MemoryUsedGB,
		MemoryUsagePercent: req.MemoryUsagePercent,
		SwapTotalGB:        req.SwapTotalGB,
		SwapUsedGB:         req.SwapUsedGB,
		SwapUsagePercent:   req.SwapUsagePercent,
		DiskUsagePercent:   req.DiskUsagePercent,
		DiskDetails:        req.DiskDetails,
		MonitoringSnapshot: req.MonitoringSnapshot,
	})
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "node not found" {
			statusCode = http.StatusNotFound
		}
		writeJSON(w, statusCode, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "node reported",
		"node":    reported,
	})
}

func (h *NodeHandler) Assign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	item, err := h.nodeService.AssignByRegionZone(
		r.Context(),
		r.URL.Query().Get("region"),
		r.URL.Query().Get("zone"),
	)
	if err != nil {
		statusCode := http.StatusInternalServerError
		switch {
		case err.Error() == "node not found":
			statusCode = http.StatusNotFound
		case isAssignValidationError(err):
			statusCode = http.StatusBadRequest
		}
		writeJSON(w, statusCode, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"node": item,
	})
}

func (h *NodeHandler) Monitoring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	uuidValue, ok := extractNodeUUIDWithSuffix(r.URL.Path, "/api/v1/nodes/", "/monitoring")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid node uuid path",
		})
		return
	}

	snapshot, err := h.nodeService.GetMonitoringSnapshot(r.Context(), uuidValue)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "node not found" {
			statusCode = http.StatusNotFound
		}
		writeJSON(w, statusCode, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"uuid":               uuidValue,
		"monitoring_snapshot": snapshot,
	})
}

func (h *NodeHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	items, err := h.nodeService.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": items,
		"count": len(items),
	})
}

func (h *NodeHandler) Detail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	uuidValue, ok := extractNodeUUID(r.URL.Path, "/api/v1/nodes/")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid node uuid path",
		})
		return
	}

	item, err := h.nodeService.GetByUUID(r.Context(), uuidValue)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "node not found" {
			statusCode = http.StatusNotFound
		}
		writeJSON(w, statusCode, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"node": item,
	})
}

func (h *NodeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	uuidValue, ok := extractNodeUUID(r.URL.Path, "/api/v1/nodes/")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid node uuid path",
		})
		return
	}

	if err := h.nodeService.DeleteByUUID(r.Context(), uuidValue); err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "node not found" {
			statusCode = http.StatusNotFound
		}
		writeJSON(w, statusCode, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "node deleted",
		"uuid":    uuidValue,
	})
}

func (h *NodeHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	uuidValue, ok := extractNodeUUIDWithSuffix(r.URL.Path, "/api/v1/nodes/", "/status")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid node uuid path",
		})
		return
	}

	var req updateNodeStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "invalid json body",
		})
		return
	}

	item, err := h.nodeService.SetStatus(r.Context(), uuidValue, req.Status)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == "node not found" {
			statusCode = http.StatusNotFound
		}
		writeJSON(w, statusCode, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "node status updated",
		"node":    item,
	})
}

func (h *NodeHandler) ListRegions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
		return
	}

	items, err := h.nodeService.ListRegionZones(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"region_zones": items,
		"count":        len(items),
	})
}

func extractNodeUUID(path, prefix string) (string, bool) {
	trimmed := strings.TrimPrefix(path, prefix)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", false
	}
	return trimmed, true
}

func extractNodeUUIDWithSuffix(path, prefix, suffix string) (string, bool) {
	if !strings.HasSuffix(path, suffix) {
		return "", false
	}
	trimmed := strings.TrimSuffix(path, suffix)
	return extractNodeUUID(trimmed, prefix)
}

func isAssignValidationError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	switch {
	case message == "region is required":
		return true
	case message == "zone is required":
		return true
	case strings.HasPrefix(message, "region \"") && strings.HasSuffix(message, "\" is not configured"):
		return true
	case strings.HasPrefix(message, "region \"") && strings.HasSuffix(message, "\" has no configured zones"):
		return true
	case strings.HasPrefix(message, "zone \"") && strings.Contains(message, "\" is not allowed for region \""):
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
