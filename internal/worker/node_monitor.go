package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/service"
)

type NodeMonitor struct {
	nodeService *service.NodeService
	timeoutSec  int
	intervalSec int
}

func NewNodeMonitor(nodeService *service.NodeService, timeoutSec, intervalSec int) *NodeMonitor {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if intervalSec <= 0 {
		intervalSec = 5
	}

	return &NodeMonitor{
		nodeService: nodeService,
		timeoutSec:  timeoutSec,
		intervalSec: intervalSec,
	}
}

func (m *NodeMonitor) Run() {
	ticker := time.NewTicker(time.Duration(m.intervalSec) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		count, err := m.nodeService.MarkTimedOutNodesDown(context.Background(), m.timeoutSec)
		if err != nil {
			log.Printf("node monitor failed: %v", err)
			continue
		}
		if count > 0 {
			log.Printf("node monitor marked %d node(s) down after %d seconds without reports", count, m.timeoutSec)
		}
	}
}
