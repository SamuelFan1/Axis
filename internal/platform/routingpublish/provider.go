package routingpublish

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/routing"
)

type Publisher interface {
	Enabled() bool
	PublishSnapshot(ctx context.Context, manifest routing.Manifest, bundles []routing.Bundle) error
}
