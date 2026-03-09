package repository

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/region"
)

type RegionRepository interface {
	EnsureSchema(ctx context.Context) error
	Create(ctx context.Context, name string) (region.Region, error)
	List(ctx context.Context) ([]region.RegionListItem, error)
	FindByUUID(ctx context.Context, uuid string) (*region.Region, error)
	FindByName(ctx context.Context, name string) (*region.Region, error)
	DeleteByUUID(ctx context.Context, uuid string) (bool, error)
	DeleteNodesByRegionUUID(ctx context.Context, regionUUID string) (int64, error)
	MigrateNodesRegionUUID(ctx context.Context) error
}
