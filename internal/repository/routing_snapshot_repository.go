package repository

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/routing"
)

type RoutingSnapshotRepository interface {
	EnsureSchema(ctx context.Context) error
	SaveManifest(ctx context.Context, manifest routing.Manifest) error
	SaveBundles(ctx context.Context, bundles []routing.Bundle) error
	GetLatestManifest(ctx context.Context) (*routing.Manifest, error)
	GetManifestByVersion(ctx context.Context, version string) (*routing.Manifest, error)
	ListBundlesByVersion(ctx context.Context, version string) ([]routing.Bundle, error)
}
