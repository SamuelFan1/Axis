package repository

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type NodeRepository interface {
	EnsureSchema(ctx context.Context) error
	FindByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error)
	Upsert(ctx context.Context, item node.Node) error
	List(ctx context.Context) ([]node.Node, error)
}
