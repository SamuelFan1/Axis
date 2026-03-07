package repository

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type NodeRepository interface {
	EnsureSchema(ctx context.Context) error
	FindByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error)
	FindByUUID(ctx context.Context, uuid string) (*node.Node, error)
	Upsert(ctx context.Context, item node.Node) error
	UpdateHeartbeat(ctx context.Context, item node.Node) error
	List(ctx context.Context) ([]node.Node, error)
	DeleteByUUID(ctx context.Context, uuid string) (bool, error)
	UpdateStatus(ctx context.Context, uuid string, status string) (bool, error)
	ListRegions(ctx context.Context) ([]node.RegionSummary, error)
	MarkTimedOutNodesDown(ctx context.Context, timeoutSec int) (int, error)
}
