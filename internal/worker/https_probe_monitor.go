package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/service"
)

type HTTPSProbeMonitor struct {
	service  *service.HTTPSProbeService
	interval time.Duration
}

func NewHTTPSProbeMonitor(probeService *service.HTTPSProbeService, intervalSec int) *HTTPSProbeMonitor {
	if intervalSec <= 0 {
		intervalSec = 5
	}
	return &HTTPSProbeMonitor{
		service:  probeService,
		interval: time.Duration(intervalSec) * time.Second,
	}
}

func (m *HTTPSProbeMonitor) Run() {
	if m == nil || m.service == nil {
		return
	}
	m.runCycle()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for range ticker.C {
		m.runCycle()
	}
}

func (m *HTTPSProbeMonitor) runCycle() {
	if err := m.service.RunCycle(context.Background()); err != nil {
		log.Printf("HTTPS probe cycle failed: %v", err)
	}
}
