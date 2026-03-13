package repository

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/observation"
)

type ObservationRepository interface {
	EnsureSchema(ctx context.Context) error
	UpsertMany(ctx context.Context, items []observation.RecordInput) error
	List(ctx context.Context) ([]observation.Aggregate, error)
}
