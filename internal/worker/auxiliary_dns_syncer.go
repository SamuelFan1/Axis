package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/service"
)

type AuxiliaryDNSSyncer struct {
	service     *service.AuxiliaryDNSService
	intervalSec int
}

func NewAuxiliaryDNSSyncer(service *service.AuxiliaryDNSService, intervalSec int) *AuxiliaryDNSSyncer {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	return &AuxiliaryDNSSyncer{
		service:     service,
		intervalSec: intervalSec,
	}
}

func (w *AuxiliaryDNSSyncer) Run() {
	if w.service == nil {
		log.Print("auxiliary dns syncer disabled: service is not configured")
		return
	}

	w.syncOnce()
	ticker := time.NewTicker(time.Duration(w.intervalSec) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		w.syncOnce()
	}
}

func (w *AuxiliaryDNSSyncer) syncOnce() {
	result, err := w.service.Sync(context.Background())
	if err != nil {
		log.Printf("auxiliary dns sync failed: %v", err)
		return
	}
	log.Printf(
		"auxiliary dns sync completed: expected=%d existing=%d created=%d updated=%d deleted=%d skipped=%d",
		result.Expected,
		result.Existing,
		result.Created,
		result.UpdatedOrEnsured,
		result.Deleted,
		result.Skipped,
	)
}
