package repository

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/zone"
)

type ZoneRepository interface {
	EnsureSchema(ctx context.Context) error
	Create(ctx context.Context, name string) (zone.Zone, error)
	List(ctx context.Context) ([]zone.ZoneListItem, error)
	FindByUUID(ctx context.Context, uuid string) (*zone.Zone, error)
	FindByName(ctx context.Context, name string) (*zone.Zone, error)
	DeleteByUUID(ctx context.Context, uuid string) (bool, error)
	DeleteNodesByZoneUUID(ctx context.Context, zoneUUID string) (int64, error)
	MigrateNodesZoneUUID(ctx context.Context) error
}
