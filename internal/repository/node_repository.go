package repository

import (
	"context"
	"encoding/json"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type NodeIdentityRepository interface {
	EnsureSchema(ctx context.Context) error
	FindActiveByManagementAddress(ctx context.Context, managementAddress string) (*node.NodeIdentity, error)
	FindIdentityByUUID(ctx context.Context, uuid string) (*node.NodeIdentity, error)
	UpsertIdentity(ctx context.Context, item node.NodeIdentity) error
	SaveDNSBinding(ctx context.Context, uuid string, label string, name string) error
	ArchiveAndDeleteByManagementAddress(ctx context.Context, managementAddress string, replacedByUUID string, reason string) error
	DeleteByUUID(ctx context.Context, uuid string) (bool, error)
	DeleteByRegionUUID(ctx context.Context, regionUUID string) (int64, error)
	DeleteByZoneUUID(ctx context.Context, zoneUUID string) (int64, error)
}

type NodeHealthRepository interface {
	EnsureSchema(ctx context.Context) error
	UpsertHealth(ctx context.Context, item node.NodeHealth) error
	FindLatestHealthByNodeUUID(ctx context.Context, nodeUUID string) (*node.NodeHealth, error)
	GetMonitoringSnapshot(ctx context.Context, nodeUUID string) (json.RawMessage, error)
	MarkTimedOutNodesDown(ctx context.Context, localRegion string, timeoutSec int) (int, error)
	CleanupOrphanedHealthRows(ctx context.Context, localRegion string) (int, error)
}

type NodeViewRepository interface {
	List(ctx context.Context) ([]node.Node, error)
	FindByUUID(ctx context.Context, uuid string) (*node.Node, error)
	ListRegions(ctx context.Context) ([]node.RegionSummary, error)
	ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error)
}
