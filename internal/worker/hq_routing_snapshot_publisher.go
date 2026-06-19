package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/service"
)

type HQRoutingSnapshotPublisher struct {
	snapshotService *service.HQRoutingSnapshotService
	publishService  *service.RoutingPublishService
	interval        time.Duration
}

func NewHQRoutingSnapshotPublisher(snapshotService *service.HQRoutingSnapshotService, publishService *service.RoutingPublishService, intervalSec int) *HQRoutingSnapshotPublisher {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	return &HQRoutingSnapshotPublisher{
		snapshotService: snapshotService,
		publishService:  publishService,
		interval:        time.Duration(intervalSec) * time.Second,
	}
}

func (w *HQRoutingSnapshotPublisher) Run() {
	if w == nil || w.snapshotService == nil || w.publishService == nil || !w.publishService.Enabled() {
		return
	}

	runOnce := func() {
		manifest, bundles, err := w.snapshotService.Generate(context.Background())
		if err != nil {
			log.Printf("hq routing snapshot generate failed: %v", err)
			return
		}
		if err := w.publishService.Publish(context.Background(), manifest, bundles); err != nil {
			log.Printf("hq routing snapshot publish failed: %v", err)
			return
		}
		log.Printf("hq routing snapshot published: key=%s version=%s bundles=%d candidates=%d", manifest.Key, manifest.Version, len(bundles), len(manifest.GlobalCandidates))
	}

	runOnce()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for range ticker.C {
		runOnce()
	}
}
