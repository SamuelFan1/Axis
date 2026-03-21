package repository

import (
	"context"
	"encoding/json"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type RegionalNodeStatusSnapshotRepository interface {
	EnsureSchema(ctx context.Context) error
	UpsertSnapshot(ctx context.Context, snapshot node.RegionalNodeStatusSnapshot) error
	ListLatestByRegion(ctx context.Context, sourceRegion string) ([]node.RegionalNodeStatus, error)
	ListLatest(ctx context.Context) ([]node.RegionalNodeStatus, error)
}

type AggregatedNodeRepository interface {
	EnsureSchema(ctx context.Context) error
	ReplaceAll(ctx context.Context, items []node.AggregatedNodeStatus) error
	List(ctx context.Context) ([]node.Node, error)
	FindByUUID(ctx context.Context, uuid string) (*node.Node, error)
	ListRegions(ctx context.Context) ([]node.RegionSummary, error)
	ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error)
	GetMonitoringSnapshot(ctx context.Context, uuid string) (json.RawMessage, error)
}
