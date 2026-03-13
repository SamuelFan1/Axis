package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/service"
)

type RoutingSnapshotPublisher struct {
	snapshotService *service.RoutingSnapshotService
	publishService  *service.RoutingPublishService
	interval        time.Duration
}

func NewRoutingSnapshotPublisher(
	snapshotService *service.RoutingSnapshotService,
	publishService *service.RoutingPublishService,
	intervalSec int,
) *RoutingSnapshotPublisher {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	return &RoutingSnapshotPublisher{
		snapshotService: snapshotService,
		publishService:  publishService,
		interval:        time.Duration(intervalSec) * time.Second,
	}
}

func (w *RoutingSnapshotPublisher) Run() {
	if w == nil || w.snapshotService == nil || w.publishService == nil || !w.publishService.Enabled() {
		return
	}

	runOnce := func() {
		manifest, bundles, err := w.snapshotService.GenerateAndStore(context.Background())
		if err != nil {
			log.Printf("routing snapshot generate failed: %v", err)
			return
		}
		if err := w.publishService.Publish(context.Background(), manifest, bundles); err != nil {
			log.Printf("routing snapshot publish failed: %v", err)
			return
		}
		log.Printf("routing snapshot published: version=%s bundles=%d", manifest.Version, len(bundles))
	}

	runOnce()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for range ticker.C {
		runOnce()
	}
}
