package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/zone"
	"github.com/SamuelFan1/Axis/internal/repository"
	"github.com/google/uuid"
)

type ZoneService struct {
	zoneRepo   repository.ZoneRepository
	regionRepo repository.RegionRepository
	config     config.RegionConfig
}

func NewZoneService(zoneRepo repository.ZoneRepository, regionRepo repository.RegionRepository, cfg config.RegionConfig) *ZoneService {
	return &ZoneService{
		zoneRepo:   zoneRepo,
		regionRepo: regionRepo,
		config:     cfg,
	}
}

func (s *ZoneService) Create(ctx context.Context, regionUUID string, name string) (zone.Zone, error) {
	regionUUID = strings.TrimSpace(regionUUID)
	name = strings.TrimSpace(strings.ToUpper(name))
	if regionUUID == "" {
		return zone.Zone{}, fmt.Errorf("region uuid is required")
	}
	if _, err := uuid.Parse(regionUUID); err != nil {
		return zone.Zone{}, fmt.Errorf("invalid region uuid")
	}
	regionItem, err := s.regionRepo.FindByUUID(ctx, regionUUID)
	if err != nil {
		return zone.Zone{}, fmt.Errorf("find region by uuid: %w", err)
	}
	if regionItem == nil {
		return zone.Zone{}, fmt.Errorf("region not found")
	}
	if err := s.config.ValidateRegionZone(regionItem.Name, name); err != nil {
		return zone.Zone{}, err
	}
	return s.zoneRepo.Create(ctx, regionUUID, name)
}

func (s *ZoneService) EnsureConfigured(ctx context.Context) error {
	for _, regionName := range s.config.Regions {
		normalizedRegion := strings.TrimSpace(strings.ToLower(regionName))
		if normalizedRegion == "" {
			continue
		}
		regionItem, err := s.regionRepo.FindByName(ctx, normalizedRegion)
		if err != nil {
			return fmt.Errorf("find configured region %q: %w", normalizedRegion, err)
		}
		if regionItem == nil {
			continue
		}
		for _, zoneName := range s.config.RegionZones[normalizedRegion] {
			normalizedZone := strings.TrimSpace(strings.ToUpper(zoneName))
			if normalizedZone == "" {
				continue
			}
			existing, err := s.zoneRepo.FindByRegionUUIDAndName(ctx, regionItem.UUID, normalizedZone)
			if err != nil {
				return fmt.Errorf("find configured zone %q for region %q: %w", normalizedZone, normalizedRegion, err)
			}
			if existing != nil {
				continue
			}
			if _, err := s.zoneRepo.Create(ctx, regionItem.UUID, normalizedZone); err != nil {
				return fmt.Errorf("create configured zone %q for region %q: %w", normalizedZone, normalizedRegion, err)
			}
		}
	}
	return nil
}

func (s *ZoneService) List(ctx context.Context) ([]zone.ZoneListItem, error) {
	return s.zoneRepo.List(ctx)
}

func (s *ZoneService) DeleteByUUID(ctx context.Context, zoneUUID string) error {
	zoneUUID = strings.TrimSpace(zoneUUID)
	if zoneUUID == "" {
		return fmt.Errorf("zone uuid is required")
	}
	if _, err := uuid.Parse(zoneUUID); err != nil {
		return fmt.Errorf("invalid zone uuid")
	}
	if _, err := s.zoneRepo.DeleteNodesByZoneUUID(ctx, zoneUUID); err != nil {
		return fmt.Errorf("delete nodes by zone: %w", err)
	}
	deleted, err := s.zoneRepo.DeleteByUUID(ctx, zoneUUID)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("zone not found")
	}
	return nil
}
