package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/service"
)

type MainDNSSyncer struct {
	service     *service.MainDNSSyncService
	intervalSec int
}

func NewMainDNSSyncer(service *service.MainDNSSyncService, intervalSec int) *MainDNSSyncer {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	return &MainDNSSyncer{
		service:     service,
		intervalSec: intervalSec,
	}
}

func (w *MainDNSSyncer) Run() {
	if w.service == nil {
		log.Print("main dns syncer disabled: service is not configured")
		return
	}

	w.syncOnce()
	ticker := time.NewTicker(time.Duration(w.intervalSec) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		w.syncOnce()
	}
}

func (w *MainDNSSyncer) syncOnce() {
	result, err := w.service.Sync(context.Background())
	if err != nil {
		log.Printf("main dns sync failed: %v", err)
		return
	}
	log.Printf(
		"main dns sync completed: expected=%d existing=%d created=%d updated=%d skipped=%d missing_binding=%d",
		result.Expected,
		result.Existing,
		result.Created,
		result.UpdatedOrEnsured,
		result.Skipped,
		result.MissingBinding,
	)
}
