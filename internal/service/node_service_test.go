package service

import (
	"context"
	"testing"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/region"
	"github.com/SamuelFan1/Axis/internal/domain/zone"
)

type stubNodeRepository struct {
	items []node.Node
	err   error
}

func (r *stubNodeRepository) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubNodeRepository) FindByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error) {
	return nil, nil
}

func (r *stubNodeRepository) FindByUUID(ctx context.Context, uuid string) (*node.Node, error) {
	return nil, nil
}

func (r *stubNodeRepository) Upsert(ctx context.Context, item node.Node) error {
	return nil
}

func (r *stubNodeRepository) UpdateHeartbeat(ctx context.Context, item node.Node) error {
	return nil
}

func (r *stubNodeRepository) EnsureDNSBinding(ctx context.Context, uuid string, prefix string, zone string) (*node.Node, error) {
	return nil, nil
}

func (r *stubNodeRepository) List(ctx context.Context) ([]node.Node, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.items, nil
}

func (r *stubNodeRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	return false, nil
}

func (r *stubNodeRepository) UpdateStatus(ctx context.Context, uuid string, status string) (bool, error) {
	return false, nil
}

func (r *stubNodeRepository) ListRegions(ctx context.Context) ([]node.RegionSummary, error) {
	return nil, nil
}

func (r *stubNodeRepository) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	return nil, nil
}

func (r *stubNodeRepository) MarkTimedOutNodesDown(ctx context.Context, localRegion string, timeoutSec int) (int, error) {
	return 0, nil
}

type stubRegionRepository struct{}

func (r *stubRegionRepository) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubRegionRepository) Create(ctx context.Context, name string) (region.Region, error) {
	return region.Region{}, nil
}

func (r *stubRegionRepository) List(ctx context.Context) ([]region.RegionListItem, error) {
	return nil, nil
}

func (r *stubRegionRepository) FindByUUID(ctx context.Context, uuid string) (*region.Region, error) {
	return nil, nil
}

func (r *stubRegionRepository) FindByName(ctx context.Context, name string) (*region.Region, error) {
	return nil, nil
}

func (r *stubRegionRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	return false, nil
}

func (r *stubRegionRepository) DeleteNodesByRegionUUID(ctx context.Context, regionUUID string) (int64, error) {
	return 0, nil
}

func (r *stubRegionRepository) MigrateNodesRegionUUID(ctx context.Context) error {
	return nil
}

type stubZoneRepository struct{}

func (r *stubZoneRepository) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubZoneRepository) Create(ctx context.Context, name string) (zone.Zone, error) {
	return zone.Zone{}, nil
}

func (r *stubZoneRepository) List(ctx context.Context) ([]zone.ZoneListItem, error) {
	return nil, nil
}

func (r *stubZoneRepository) FindByUUID(ctx context.Context, uuid string) (*zone.Zone, error) {
	return nil, nil
}

func (r *stubZoneRepository) FindByName(ctx context.Context, name string) (*zone.Zone, error) {
	return nil, nil
}

func (r *stubZoneRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	return false, nil
}

func (r *stubZoneRepository) DeleteNodesByZoneUUID(ctx context.Context, zoneUUID string) (int64, error) {
	return 0, nil
}

func (r *stubZoneRepository) MigrateNodesZoneUUID(ctx context.Context) error {
	return nil
}

func newTestNodeService(items []node.Node) *NodeService {
	return &NodeService{
		repo:       &stubNodeRepository{items: items},
		regionRepo: &stubRegionRepository{},
		zoneRepo:   &stubZoneRepository{},
		regionConfig: config.RegionConfig{
			Regions: []string{"asia", "europe"},
			RegionZones: map[string][]string{
				"asia":   {"SG", "JP"},
				"europe": {"DE"},
			},
		},
	}
}

func TestAssignByRegionZonePrefersZoneLowestScore(t *testing.T) {
	svc := newTestNodeService([]node.Node{
		{UUID: "zone-high", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 90, CPUUsagePercent: 50, MemoryUsagePercent: 40},
		{UUID: "zone-low", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 30, CPUUsagePercent: 20, MemoryUsagePercent: 10},
		{UUID: "region-lower", Region: "asia", Zone: "JP", Status: node.StatusUp, DiskUsagePercent: 1, CPUUsagePercent: 1, MemoryUsagePercent: 1},
	})

	item, err := svc.AssignByRegionZone(context.Background(), "asia", "SG")
	if err != nil {
		t.Fatalf("AssignByRegionZone returned error: %v", err)
	}
	if item.UUID != "zone-low" {
		t.Fatalf("expected zone-low, got %s", item.UUID)
	}
}

func TestAssignByRegionZoneFallsBackToRegion(t *testing.T) {
	svc := newTestNodeService([]node.Node{
		{UUID: "zone-down", Region: "asia", Zone: "SG", Status: node.StatusDown, DiskUsagePercent: 10, CPUUsagePercent: 10, MemoryUsagePercent: 10},
		{UUID: "region-up", Region: "asia", Zone: "JP", Status: node.StatusUp, DiskUsagePercent: 20, CPUUsagePercent: 20, MemoryUsagePercent: 20},
	})

	item, err := svc.AssignByRegionZone(context.Background(), "asia", "SG")
	if err != nil {
		t.Fatalf("AssignByRegionZone returned error: %v", err)
	}
	if item.UUID != "region-up" {
		t.Fatalf("expected region-up, got %s", item.UUID)
	}
}

func TestAssignByRegionZoneReturnsNotFoundWithoutUpNodes(t *testing.T) {
	svc := newTestNodeService([]node.Node{
		{UUID: "zone-down", Region: "asia", Zone: "SG", Status: node.StatusDown},
		{UUID: "region-down", Region: "asia", Zone: "JP", Status: node.StatusDown},
		{UUID: "other-region-up", Region: "europe", Zone: "DE", Status: node.StatusUp},
	})

	_, err := svc.AssignByRegionZone(context.Background(), "asia", "SG")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "node not found" {
		t.Fatalf("expected node not found, got %v", err)
	}
}

func TestAssignByRegionZoneReturnsOneOfLowestScoreTies(t *testing.T) {
	svc := newTestNodeService([]node.Node{
		{UUID: "best-a", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 20, CPUUsagePercent: 20, MemoryUsagePercent: 20},
		{UUID: "best-b", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 20, CPUUsagePercent: 20, MemoryUsagePercent: 20},
		{UUID: "worse", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 60, CPUUsagePercent: 60, MemoryUsagePercent: 60},
	})

	item, err := svc.AssignByRegionZone(context.Background(), "asia", "SG")
	if err != nil {
		t.Fatalf("AssignByRegionZone returned error: %v", err)
	}
	if item.UUID != "best-a" && item.UUID != "best-b" {
		t.Fatalf("expected one of best-a or best-b, got %s", item.UUID)
	}
}

func TestAssignByRegionZoneValidatesRegionZone(t *testing.T) {
	svc := newTestNodeService(nil)

	_, err := svc.AssignByRegionZone(context.Background(), "asia", "CN")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != `zone "CN" is not allowed for region "asia"` {
		t.Fatalf("unexpected error: %v", err)
	}
}
