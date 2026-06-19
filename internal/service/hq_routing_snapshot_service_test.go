package service

import (
	"context"
	"testing"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/observation"
	"github.com/SamuelFan1/Axis/internal/domain/region"
	"github.com/SamuelFan1/Axis/internal/domain/routing"
	"github.com/SamuelFan1/Axis/internal/domain/zone"
)

func TestHQRoutingSnapshotPublishesOnlyReadyHQNodes(t *testing.T) {
	service := NewHQRoutingSnapshotService(
		hqObservationRepo{items: []observation.Aggregate{
			{
				SourceColo:          "SIN",
				TargetNodeUUID:      "node-184",
				SuccessLatencySumMs: 80,
				SuccessCount:        2,
				SampleCount:         2,
				LastObservedAt:      time.Now().UTC(),
			},
			{
				SourceColo:          "SIN",
				TargetNodeUUID:      "node-185",
				SuccessLatencySumMs: 10,
				SuccessCount:        1,
				SampleCount:         1,
				LastObservedAt:      time.Now().UTC(),
			},
		}},
		stubAuxiliaryNodeViewRepo{items: []node.Node{
			{
				UUID:               "node-184",
				Hostname:           "SGP-CONTABO-4V8G-SERVER-009",
				PublicIP:           "109.123.232.41",
				Region:             "asia",
				Zone:               "SG",
				Status:             node.StatusUp,
				MonitoringSnapshot: hqReadyMonitoringSnapshot(),
			},
			{
				UUID:               "node-185",
				Hostname:           "NOSESSION-CONTABO",
				PublicIP:           "203.0.113.2",
				Region:             "asia",
				Zone:               "SG",
				Status:             node.StatusUp,
				MonitoringSnapshot: hqSessionMissingMonitoringSnapshot(),
			},
			{
				UUID:               "node-186",
				Hostname:           "DOWN-CONTABO",
				PublicIP:           "203.0.113.3",
				Region:             "asia",
				Zone:               "SG",
				Status:             node.StatusDown,
				MonitoringSnapshot: hqReadyMonitoringSnapshot(),
			},
		}},
		hqRegionRepo{items: []region.RegionListItem{{Name: "asia"}}},
		hqZoneRepo{items: []zone.ZoneListItem{{RegionName: "asia", Name: "SG"}}},
		&stubDNSBindingRepository{bindings: map[string]dnsbinding.Binding{
			"node-184": {
				NodeUUID:     "node-184",
				DNSLabel:     "dl-184",
				DNSName:      "dl-184.nuxdisk.com",
				Zone:         "nuxdisk.com",
				RecordPrefix: "dl-",
			},
			"node-185": {
				NodeUUID:     "node-185",
				DNSLabel:     "dl-185",
				DNSName:      "dl-185.nuxdisk.com",
				Zone:         "nuxdisk.com",
				RecordPrefix: "dl-",
			},
			"node-186": {
				NodeUUID:     "node-186",
				DNSLabel:     "dl-186",
				DNSName:      "dl-186.nuxdisk.com",
				Zone:         "nuxdisk.com",
				RecordPrefix: "dl-",
			},
		}},
		config.RoutingConfig{
			SnapshotTTLSeconds: 90,
			TopN:               3,
		},
		testHQConfig(),
	)

	manifest, bundles, err := service.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if manifest.Key != "hq:routing:manifest" {
		t.Fatalf("expected HQ manifest key, got %q", manifest.Key)
	}
	if len(manifest.GlobalCandidates) != 1 {
		t.Fatalf("expected one global candidate, got %+v", manifest.GlobalCandidates)
	}
	assertHQCandidate(t, manifest.GlobalCandidates[0])

	zoneCandidates := manifest.ZoneCandidates["SG"]
	if len(zoneCandidates) != 1 {
		t.Fatalf("expected one SG candidate, got %+v", zoneCandidates)
	}
	assertHQCandidate(t, zoneCandidates[0])

	regionCandidates := manifest.RegionCandidates["asia"]
	if len(regionCandidates) != 1 {
		t.Fatalf("expected one asia candidate, got %+v", regionCandidates)
	}
	assertHQCandidate(t, regionCandidates[0])

	if len(bundles) != 1 {
		t.Fatalf("expected one bundle, got %+v", bundles)
	}
	if bundles[0].Key != "hq:routing:bundle:asia" {
		t.Fatalf("expected HQ bundle key, got %q", bundles[0].Key)
	}
	coloCandidates := bundles[0].Entries["SIN"]
	if len(coloCandidates) != 1 {
		t.Fatalf("expected one SIN candidate, got %+v", coloCandidates)
	}
	assertHQCandidate(t, coloCandidates[0])
}

func assertHQCandidate(t *testing.T, candidate routing.Candidate) {
	t.Helper()
	if candidate.NodeUUID != "node-184" || candidate.ServiceHost != "n184.aiplexlink.com" || candidate.Region != "asia" || candidate.Zone != "SG" {
		t.Fatalf("unexpected HQ candidate: %+v", candidate)
	}
}

type hqObservationRepo struct {
	items []observation.Aggregate
}

func (r hqObservationRepo) EnsureSchema(context.Context) error {
	return nil
}

func (r hqObservationRepo) UpsertMany(context.Context, []observation.RecordInput) error {
	return nil
}

func (r hqObservationRepo) List(context.Context) ([]observation.Aggregate, error) {
	return append([]observation.Aggregate(nil), r.items...), nil
}

type hqRegionRepo struct {
	items []region.RegionListItem
}

func (r hqRegionRepo) EnsureSchema(context.Context) error {
	return nil
}

func (r hqRegionRepo) Create(context.Context, string) (region.Region, error) {
	return region.Region{}, nil
}

func (r hqRegionRepo) List(context.Context) ([]region.RegionListItem, error) {
	return append([]region.RegionListItem(nil), r.items...), nil
}

func (r hqRegionRepo) FindByUUID(context.Context, string) (*region.Region, error) {
	return nil, nil
}

func (r hqRegionRepo) FindByName(context.Context, string) (*region.Region, error) {
	return nil, nil
}

func (r hqRegionRepo) DeleteByUUID(context.Context, string) (bool, error) {
	return false, nil
}

func (r hqRegionRepo) DeleteNodesByRegionUUID(context.Context, string) (int64, error) {
	return 0, nil
}

func (r hqRegionRepo) MigrateNodesRegionUUID(context.Context) error {
	return nil
}

type hqZoneRepo struct {
	items []zone.ZoneListItem
}

func (r hqZoneRepo) EnsureSchema(context.Context) error {
	return nil
}

func (r hqZoneRepo) EnsureConstraints(context.Context) error {
	return nil
}

func (r hqZoneRepo) Create(context.Context, string, string) (zone.Zone, error) {
	return zone.Zone{}, nil
}

func (r hqZoneRepo) List(context.Context) ([]zone.ZoneListItem, error) {
	return append([]zone.ZoneListItem(nil), r.items...), nil
}

func (r hqZoneRepo) FindByUUID(context.Context, string) (*zone.Zone, error) {
	return nil, nil
}

func (r hqZoneRepo) FindByRegionUUIDAndName(context.Context, string, string) (*zone.Zone, error) {
	return nil, nil
}

func (r hqZoneRepo) CountByRegionUUID(context.Context, string) (int, error) {
	return 0, nil
}

func (r hqZoneRepo) DeleteByUUID(context.Context, string) (bool, error) {
	return false, nil
}

func (r hqZoneRepo) DeleteNodesByZoneUUID(context.Context, string) (int64, error) {
	return 0, nil
}

func (r hqZoneRepo) MigrateNodesZoneUUID(context.Context) error {
	return nil
}
