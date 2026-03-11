package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/region"
	"github.com/SamuelFan1/Axis/internal/repository"
	"github.com/google/uuid"
)

type RegionService struct {
	regionRepo repository.RegionRepository
	nodeRepo   repository.NodeRepository
	config     config.RegionConfig
}

func NewRegionService(regionRepo repository.RegionRepository, nodeRepo repository.NodeRepository, cfg config.RegionConfig) *RegionService {
	return &RegionService{
		regionRepo: regionRepo,
		nodeRepo:   nodeRepo,
		config:     cfg,
	}
}

func (s *RegionService) Create(ctx context.Context, name string) (region.Region, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if err := s.config.ValidateRegion(name); err != nil {
		return region.Region{}, err
	}
	return s.regionRepo.Create(ctx, name)
}

func (s *RegionService) EnsureConfigured(ctx context.Context) error {
	for _, name := range s.config.Regions {
		normalized := strings.TrimSpace(strings.ToLower(name))
		if normalized == "" {
			continue
		}
		existing, err := s.regionRepo.FindByName(ctx, normalized)
		if err != nil {
			return fmt.Errorf("find configured region %q: %w", normalized, err)
		}
		if existing != nil {
			continue
		}
		if _, err := s.regionRepo.Create(ctx, normalized); err != nil {
			return fmt.Errorf("create configured region %q: %w", normalized, err)
		}
	}
	return nil
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
