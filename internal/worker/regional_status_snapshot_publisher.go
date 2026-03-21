package worker

import (
	"context"
	"log"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/service"
)

type RegionalSnapshotSink interface {
	PublishRegionalSnapshot(ctx context.Context, snapshot node.RegionalNodeStatusSnapshot) error
}

type RegionalStatusSnapshotPublisher struct {
	snapshotService *service.RegionalStatusSnapshotService
	sink            RegionalSnapshotSink
	interval        time.Duration
}

func NewRegionalStatusSnapshotPublisher(snapshotService *service.RegionalStatusSnapshotService, sink RegionalSnapshotSink, intervalSec int) *RegionalStatusSnapshotPublisher {
	if intervalSec <= 0 {
		intervalSec = 15
	}
	return &RegionalStatusSnapshotPublisher{
		snapshotService: snapshotService,
		sink:            sink,
		interval:        time.Duration(intervalSec) * time.Second,
	}
}

func (w *RegionalStatusSnapshotPublisher) Run() {
	if w == nil || w.snapshotService == nil || w.sink == nil {
		return
	}
	runOnce := func() {
		snapshot, err := w.snapshotService.Generate(context.Background())
		if err != nil {
			log.Printf("regional snapshot generate failed: %v", err)
			return
		}
		if err := w.sink.PublishRegionalSnapshot(context.Background(), snapshot); err != nil {
			log.Printf("regional snapshot publish failed: %v", err)
			return
		}
		log.Printf("regional snapshot published: region=%s nodes=%d version=%s", snapshot.SourceRegion, len(snapshot.Nodes), snapshot.SnapshotVersion)
	}
	runOnce()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for range ticker.C {
		runOnce()
	}
}
