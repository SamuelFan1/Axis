package http

import (
	"log"
	"net/http"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/service"
)

type Server struct {
	address       string
	authConfig    config.AuthConfig
	nodeHandler   *NodeHandler
	regionHandler *RegionHandler
	zoneHandler   *ZoneHandler
}

func NewServer(address string, authConfig config.AuthConfig, nodeService *service.NodeService, regionService *service.RegionService, zoneService *service.ZoneService) *Server {
	return &Server{
		address:       address,
		authConfig:    authConfig,
		nodeHandler:   NewNodeHandler(nodeService),
		regionHandler: NewRegionHandler(regionService),
		zoneHandler:   NewZoneHandler(zoneService),
	}
}

func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/nodes/register", nodeTokenMiddleware(s.authConfig, s.nodeHandler.Register))
	mux.HandleFunc("/api/v1/nodes/report", nodeTokenMiddleware(s.authConfig, s.nodeHandler.Report))
	mux.HandleFunc("/api/v1/admin/nodes/register", adminAuthMiddleware(s.authConfig, s.nodeHandler.RegisterAdmin))
	mux.HandleFunc("/api/v1/nodes", adminAuthMiddleware(s.authConfig, s.nodeHandler.List))
	mux.HandleFunc("/api/v1/regions", adminAuthMiddleware(s.authConfig, s.routeRegions))
	mux.HandleFunc("/api/v1/regions/", adminAuthMiddleware(s.authConfig, s.routeRegionByUUID))
	mux.HandleFunc("/api/v1/zones", adminAuthMiddleware(s.authConfig, s.routeZones))
	mux.HandleFunc("/api/v1/zones/", adminAuthMiddleware(s.authConfig, s.routeZoneByUUID))
	mux.HandleFunc("/api/v1/nodes/", adminAuthMiddleware(s.authConfig, s.routeNodeByUUID))

	log.Printf("axisd listening on %s", s.address)
	return http.ListenAndServe(s.address, mux)
}

func (s *Server) routeNodeByUUID(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet:
		s.nodeHandler.Detail(w, r)
	case r.Method == http.MethodDelete:
		s.nodeHandler.Delete(w, r)
	case r.Method == http.MethodPost && hasStatusSuffix(r.URL.Path):
		s.nodeHandler.UpdateStatus(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "method not allowed",
		})
	}
}

func (s *Server) routeRegions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.regionHandler.List(w, r)
	case http.MethodPost:
		s.regionHandler.Create(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
	}
}

func (s *Server) routeZones(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.zoneHandler.List(w, r)
	case http.MethodPost:
		s.zoneHandler.Create(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
	}
}

func (s *Server) routeZoneByUUID(w http.ResponseWriter, r *http.Request) {
	uuidValue, ok := extractUUID(r.URL.Path, "/api/v1/zones/")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid zone uuid path"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		s.zoneHandler.Delete(w, r, uuidValue)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
	}
}

func (s *Server) routeRegionByUUID(w http.ResponseWriter, r *http.Request) {
	uuidValue, ok := extractUUID(r.URL.Path, "/api/v1/regions/")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid region uuid path"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		s.regionHandler.Delete(w, r, uuidValue)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
	}
}

func extractUUID(path, prefix string) (string, bool) {
	trimmed := strings.TrimPrefix(path, prefix)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", false
	}
	return trimmed, true
}

func hasStatusSuffix(path string) bool {
	return len(path) > len("/status") && path[len(path)-len("/status"):] == "/status"
}
