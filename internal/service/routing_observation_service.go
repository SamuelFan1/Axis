package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/observation"
	"github.com/SamuelFan1/Axis/internal/repository"
	"github.com/google/uuid"
)

type RoutingObservationService struct {
	repo repository.ObservationRepository
}

func NewRoutingObservationService(repo repository.ObservationRepository) *RoutingObservationService {
	return &RoutingObservationService{repo: repo}
}

func (s *RoutingObservationService) EnsureSchema(ctx context.Context) error {
	return s.repo.EnsureSchema(ctx)
}

func (s *RoutingObservationService) Record(ctx context.Context, items []observation.RecordInput) error {
	normalized := make([]observation.RecordInput, 0, len(items))
	for _, item := range items {
		item.SourceColo = strings.TrimSpace(strings.ToUpper(item.SourceColo))
		item.TargetNodeUUID = strings.TrimSpace(item.TargetNodeUUID)
		if item.SourceColo == "" {
			return fmt.Errorf("source_colo is required")
		}
		if item.TargetNodeUUID == "" {
			return fmt.Errorf("target_node_uuid is required")
		}
		if _, err := uuid.Parse(item.TargetNodeUUID); err != nil {
			return fmt.Errorf("target_node_uuid must be a valid UUID")
		}
		if item.SampleCount <= 0 {
			item.SampleCount = 1
		}
		if item.ObservedAt.IsZero() {
			item.ObservedAt = time.Now().UTC()
		}
		normalized = append(normalized, item)
	}

	return s.repo.UpsertMany(ctx, normalized)
}
