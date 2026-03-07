package http

import (
	"log"
	"net/http"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/service"
)

type Server struct {
	address     string
	authConfig  config.AuthConfig
	nodeHandler *NodeHandler
}

func NewServer(address string, authConfig config.AuthConfig, nodeService *service.NodeService) *Server {
	return &Server{
		address:     address,
		authConfig:  authConfig,
		nodeHandler: NewNodeHandler(nodeService),
	}
}

func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/nodes/register", nodeTokenMiddleware(s.authConfig, s.nodeHandler.Register))
	mux.HandleFunc("/api/v1/nodes", adminAuthMiddleware(s.authConfig, s.nodeHandler.List))

	log.Printf("axisd listening on %s", s.address)
	return http.ListenAndServe(s.address, mux)
}
