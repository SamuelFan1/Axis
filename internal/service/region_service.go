package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/domain/region"
	"github.com/SamuelFan1/Axis/internal/repository"
	"github.com/google/uuid"
)

type RegionService struct {
	regionRepo repository.RegionRepository
	nodeRepo   repository.NodeRepository
}

func NewRegionService(regionRepo repository.RegionRepository, nodeRepo repository.NodeRepository) *RegionService {
	return &RegionService{
		regionRepo: regionRepo,
		nodeRepo:   nodeRepo,
	}
}

func (s *RegionService) Create(ctx context.Context, name string) (region.Region, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return region.Region{}, fmt.Errorf("region name is required")
	}
	return s.regionRepo.Create(ctx, name)
}

func (s *RegionService) List(ctx context.Context) ([]region.RegionListItem, error) {
	return s.regionRepo.List(ctx)
}

func (s *RegionService) DeleteByUUID(ctx context.Context, regionUUID string) error {
	regionUUID = strings.TrimSpace(regionUUID)
	if regionUUID == "" {
		return fmt.Errorf("region uuid is required")
	}
	if _, err := uuid.Parse(regionUUID); err != nil {
		return fmt.Errorf("invalid region uuid")
	}
	// Delete associated nodes first (cascade)
	if _, err := s.regionRepo.DeleteNodesByRegionUUID(ctx, regionUUID); err != nil {
		return fmt.Errorf("delete nodes by region: %w", err)
	}
	deleted, err := s.regionRepo.DeleteByUUID(ctx, regionUUID)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("region not found")
	}
	return nil
}
