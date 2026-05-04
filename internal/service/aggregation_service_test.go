package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type stubSnapshotViewRepo struct {
	items []node.Node
}

func (r *stubSnapshotViewRepo) List(ctx context.Context) ([]node.Node, error) {
	return append([]node.Node(nil), r.items...), nil
}

func (r *stubSnapshotViewRepo) FindByUUID(ctx context.Context, uuid string) (*node.Node, error) {
	for _, item := range r.items {
		if item.UUID == uuid {
			copied := item
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *stubSnapshotViewRepo) ListRegions(ctx context.Context) ([]node.RegionSummary, error) {
	return nil, nil
}

func (r *stubSnapshotViewRepo) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	return nil, nil
}

type stubRegionalSnapshotRepo struct {
	items []node.RegionalNodeStatus
}

func (r *stubRegionalSnapshotRepo) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubRegionalSnapshotRepo) UpsertSnapshot(ctx context.Context, snapshot node.RegionalNodeStatusSnapshot) error {
	r.items = append(r.items[:0], snapshot.Nodes...)
	return nil
}

func (r *stubRegionalSnapshotRepo) ListLatestByRegion(ctx context.Context, sourceRegion string) ([]node.RegionalNodeStatus, error) {
	var filtered []node.RegionalNodeStatus
	for _, item := range r.items {
		if item.SourceRegion == sourceRegion {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (r *stubRegionalSnapshotRepo) ListLatest(ctx context.Context) ([]node.RegionalNodeStatus, error) {
	return append([]node.RegionalNodeStatus(nil), r.items...), nil
}

type stubAggregatedRepo struct {
	items []node.AggregatedNodeStatus
}

func (r *stubAggregatedRepo) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubAggregatedRepo) ReplaceAll(ctx context.Context, items []node.AggregatedNodeStatus) error {
	r.items = append(r.items[:0], items...)
	return nil
}

func (r *stubAggregatedRepo) List(ctx context.Context) ([]node.Node, error) {
	out := make([]node.Node, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, item.Node)
	}
	return out, nil
}

func (r *stubAggregatedRepo) FindByUUID(ctx context.Context, uuid string) (*node.Node, error) {
	for _, item := range r.items {
		if item.UUID == uuid {
			copied := item.Node
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *stubAggregatedRepo) ListRegions(ctx context.Context) ([]node.RegionSummary, error) {
	return nil, nil
}

func (r *stubAggregatedRepo) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	return nil, nil
}

func (r *stubAggregatedRepo) GetMonitoringSnapshot(ctx context.Context, uuid string) (json.RawMessage, error) {
	for _, item := range r.items {
		if item.UUID == uuid {
			return append(json.RawMessage(nil), item.MonitoringSnapshot...), nil
		}
	}
	return nil, nil
}

func TestRegionalStatusSnapshotServiceGenerateFiltersLocalRegion(t *testing.T) {
	now := time.Now().UTC()
	service := NewRegionalStatusSnapshotService(&stubSnapshotViewRepo{
		items: []node.Node{
			{UUID: "asia-node", Region: "asia", Zone: "SG", Status: node.StatusUp, LastReportedAt: now},
			{UUID: "eu-node", Region: "europe", Zone: "DE", Status: node.StatusUp, LastReportedAt: now},
		},
	}, "asia")

	snapshot, err := service.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if snapshot.SourceRegion != "asia" {
		t.Fatalf("expected source region asia, got %s", snapshot.SourceRegion)
	}
	if len(snapshot.Nodes) != 1 {
		t.Fatalf("expected 1 local-region node, got %d", len(snapshot.Nodes))
	}
	if snapshot.Nodes[0].NodeUUID != "asia-node" {
		t.Fatalf("expected asia-node, got %+v", snapshot.Nodes[0])
	}
}

func TestAggregatedNodeServiceRebuildUsesHomeRegionSnapshots(t *testing.T) {
	now := time.Now().UTC()
	baseRepo := &stubSnapshotViewRepo{
		items: []node.Node{
			{UUID: "asia-node", Hostname: "SGP-01", Region: "asia", Zone: "SG", Status: node.StatusDown},
			{UUID: "eu-node", Hostname: "DE-01", Region: "europe", Zone: "DE", Status: node.StatusDown},
		},
	}
	snapshotRepo := &stubRegionalSnapshotRepo{
		items: []node.RegionalNodeStatus{
			{
				NodeUUID:           "asia-node",
				HomeRegion:         "asia",
				SourceRegion:       "asia",
				Status:             node.StatusUp,
				InternalIP:         "10.0.0.1",
				PublicIP:           "1.1.1.1",
				MonitoringSnapshot: json.RawMessage(`{"sources":[{"name":"cloudflared","status":"ok"}]}`),
				LastReportedAt:     now,
				ObservedAt:         now,
			},
		},
	}
	aggregatedRepo := &stubAggregatedRepo{}
	service := NewAggregatedNodeService(baseRepo, snapshotRepo, aggregatedRepo, 90, nil)

	count, err := service.Rebuild(context.Background())
	if err != nil {
		t.Fatalf("Rebuild returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 aggregated rows, got %d", count)
	}
	if len(aggregatedRepo.items) != 2 {
		t.Fatalf("expected 2 stored aggregated rows, got %d", len(aggregatedRepo.items))
	}

	var asiaRow, euRow node.AggregatedNodeStatus
	for _, item := range aggregatedRepo.items {
		switch item.UUID {
		case "asia-node":
			asiaRow = item
		case "eu-node":
			euRow = item
		}
	}
	if asiaRow.Status != node.StatusUp || asiaRow.Stale {
		t.Fatalf("expected asia node up and fresh, got %+v", asiaRow)
	}
	if euRow.Status != node.StatusDown || !euRow.Stale {
		t.Fatalf("expected eu node down and stale without snapshot, got %+v", euRow)
	}
	if asiaRow.PublicIP != "1.1.1.1" {
		t.Fatalf("expected asia node public ip from snapshot, got %+v", asiaRow)
	}
}

func TestNodeServicePrefersGlobalAggregatedReadModel(t *testing.T) {
	localRepo := &stubNodeRepository{
		nodes: map[string]node.Node{
			testNodeUUID: {
				UUID:   testNodeUUID,
				Region: "asia",
				Zone:   "SG",
				Status: node.StatusDown,
			},
		},
	}
	globalRepo := &stubAggregatedRepo{
		items: []node.AggregatedNodeStatus{
			{
				Node: node.Node{
					UUID:   testNodeUUID,
					Region: "asia",
					Zone:   "SG",
					Status: node.StatusUp,
				},
				HomeRegion:         "asia",
				StatusSourceRegion: "asia",
			},
		},
	}
	svc := NewNodeService(
		localRepo,
		localRepo,
		localRepo,
		globalRepo,
		&stubRegionRepository{},
		&stubZoneRepository{},
		nil,
		nil,
		config.DNSConfig{},
		config.RegionConfig{
			Regions: []string{"asia"},
			RegionZones: map[string][]string{
				"asia": {"SG"},
			},
		},
		&stubWorkerAdminClient{enabled: true},
	)

	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 || items[0].Status != node.StatusUp {
		t.Fatalf("expected global read model status up, got %+v", items)
	}

	assigned, err := svc.AssignByRegionZone(context.Background(), "asia", "SG", nil)
	if err != nil {
		t.Fatalf("AssignByRegionZone returned error: %v", err)
	}
	if assigned.Status != node.StatusUp {
		t.Fatalf("expected assignment to use global read model, got %+v", assigned)
	}
}
