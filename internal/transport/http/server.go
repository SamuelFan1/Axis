package http

import (
	"log"
	"net/http"

	"github.com/SamuelFan1/Axis/internal/service"
)

type Server struct {
	address     string
	nodeHandler *NodeHandler
}

func NewServer(address string, nodeService *service.NodeService) *Server {
	return &Server{
		address:     address,
		nodeHandler: NewNodeHandler(nodeService),
	}
}

func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/nodes/register", s.nodeHandler.Register)
	mux.HandleFunc("/api/v1/nodes", s.nodeHandler.List)

	log.Printf("axisd listening on %s", s.address)
	return http.ListenAndServe(s.address, mux)
}
