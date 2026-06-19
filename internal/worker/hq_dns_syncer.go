package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/service"
)

type HQDNSSyncer struct {
	service     *service.HQDNSService
	intervalSec int
}

func NewHQDNSSyncer(service *service.HQDNSService, intervalSec int) *HQDNSSyncer {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	return &HQDNSSyncer{
		service:     service,
		intervalSec: intervalSec,
	}
}

func (w *HQDNSSyncer) Run() {
	if w.service == nil {
		log.Print("hq dns syncer disabled: service is not configured")
		return
	}

	w.syncOnce()
	ticker := time.NewTicker(time.Duration(w.intervalSec) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		w.syncOnce()
	}
}

func (w *HQDNSSyncer) syncOnce() {
	result, err := w.service.Sync(context.Background())
	if err != nil {
		log.Printf("hq dns sync failed: %v", err)
		return
	}
	log.Printf(
		"hq dns sync completed: expected=%d ensured=%d skipped=%d missing_binding=%d",
		result.Expected,
		result.Ensured,
		result.Skipped,
		result.MissingBinding,
	)
}
