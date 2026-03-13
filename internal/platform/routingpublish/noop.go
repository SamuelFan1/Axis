package routingpublish

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/routing"
)

type NoopPublisher struct{}

func NewNoopPublisher() Publisher {
	return &NoopPublisher{}
}

func (p *NoopPublisher) Enabled() bool {
	return false
}

func (p *NoopPublisher) PublishSnapshot(ctx context.Context, manifest routing.Manifest, bundles []routing.Bundle) error {
	return nil
}
