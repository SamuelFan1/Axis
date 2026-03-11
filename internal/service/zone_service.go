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
	zoneRepo repository.ZoneRepository
	nodeRepo repository.NodeRepository
	config   config.RegionConfig
}

func NewZoneService(zoneRepo repository.ZoneRepository, nodeRepo repository.NodeRepository, cfg config.RegionConfig) *ZoneService {
	return &ZoneService{
		zoneRepo: zoneRepo,
		nodeRepo: nodeRepo,
		config:   cfg,
	}
}

func (s *ZoneService) Create(ctx context.Context, name string) (zone.Zone, error) {
	name = strings.TrimSpace(strings.ToUpper(name))
	if err := s.config.ValidateZone(name); err != nil {
		return zone.Zone{}, err
	}
	return s.zoneRepo.Create(ctx, name)
}

func (s *ZoneService) EnsureConfigured(ctx context.Context) error {
	for _, name := range s.config.AllZones() {
		normalized := strings.TrimSpace(strings.ToUpper(name))
		if normalized == "" {
			continue
		}
		existing, err := s.zoneRepo.FindByName(ctx, normalized)
		if err != nil {
			return fmt.Errorf("find configured zone %q: %w", normalized, err)
		}
		if existing != nil {
			continue
		}
		if _, err := s.zoneRepo.Create(ctx, normalized); err != nil {
			return fmt.Errorf("create configured zone %q: %w", normalized, err)
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
