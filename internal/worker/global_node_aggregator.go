package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/service"
)

type GlobalNodeAggregator struct {
	service  *service.AggregatedNodeService
	interval time.Duration
}

func NewGlobalNodeAggregator(service *service.AggregatedNodeService, intervalSec int) *GlobalNodeAggregator {
	if intervalSec <= 0 {
		intervalSec = 15
	}
	return &GlobalNodeAggregator{
		service:  service,
		interval: time.Duration(intervalSec) * time.Second,
	}
}

func (w *GlobalNodeAggregator) Run() {
	if w == nil || w.service == nil {
		return
	}
	runOnce := func() {
		count, err := w.service.Rebuild(context.Background())
		if err != nil {
			log.Printf("global node aggregation failed: %v", err)
			return
		}
		log.Printf("global node aggregation rebuilt: nodes=%d", count)
	}
	runOnce()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for range ticker.C {
		runOnce()
	}
}
