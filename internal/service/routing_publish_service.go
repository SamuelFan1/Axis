package service

import (
	"context"

	"github.com/SamuelFan1/Axis/internal/domain/routing"
	"github.com/SamuelFan1/Axis/internal/platform/routingpublish"
)

type RoutingPublishService struct {
	publisher routingpublish.Publisher
}

func NewRoutingPublishService(publisher routingpublish.Publisher) *RoutingPublishService {
	return &RoutingPublishService{publisher: publisher}
}

func (s *RoutingPublishService) Enabled() bool {
	return s != nil && s.publisher != nil && s.publisher.Enabled()
}

func (s *RoutingPublishService) Publish(ctx context.Context, manifest routing.Manifest, bundles []routing.Bundle) error {
	if !s.Enabled() {
		return nil
	}
	return s.publisher.PublishSnapshot(ctx, manifest, bundles)
}
