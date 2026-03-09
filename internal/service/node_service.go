package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	"github.com/SamuelFan1/Axis/internal/repository"
	"github.com/google/uuid"
)

type NodeService struct {
	repo         repository.NodeRepository
	regionRepo   repository.RegionRepository
	zoneRepo     repository.ZoneRepository
	dnsProvider  platformdns.Provider
	dnsConfig    config.DNSConfig
	regionConfig config.RegionConfig
}

func NewNodeService(repo repository.NodeRepository, regionRepo repository.RegionRepository, zoneRepo repository.ZoneRepository, dnsProvider platformdns.Provider, dnsConfig config.DNSConfig, regionConfig config.RegionConfig) *NodeService {
	return &NodeService{
		repo:         repo,
		regionRepo:   regionRepo,
		zoneRepo:     zoneRepo,
		dnsProvider:  dnsProvider,
		dnsConfig:    dnsConfig,
		regionConfig: regionConfig,
	}
}

func (s *NodeService) EnsureSchema(ctx context.Context) error {
	return s.repo.EnsureSchema(ctx)
}

func (s *NodeService) Register(ctx context.Context, item node.Node) (node.Node, error) {
	item.Hostname = strings.TrimSpace(item.Hostname)
	item.ManagementAddress = strings.TrimSpace(item.ManagementAddress)
	item.Region = strings.TrimSpace(strings.ToLower(item.Region))
	item.Zone = strings.TrimSpace(strings.ToUpper(item.Zone))
	item.Status = strings.ToLower(strings.TrimSpace(item.Status))

	if item.Hostname == "" {
		return node.Node{}, fmt.Errorf("hostname is required")
	}
	if item.ManagementAddress == "" {
		return node.Node{}, fmt.Errorf("management_address is required")
	}
	if item.Region == "" {
		return node.Node{}, fmt.Errorf("region is required")
	}
	if item.Zone == "" {
		return node.Node{}, fmt.Errorf("zone is required")
	}
	r, err := s.regionRepo.FindByName(ctx, item.Region)
	if err != nil {
		return node.Node{}, fmt.Errorf("find region: %w", err)
	}
	if r == nil {
		return node.Node{}, fmt.Errorf("region %q not found", item.Region)
	}
	item.RegionUUID = r.UUID
	z, err := s.zoneRepo.FindByName(ctx, item.Zone)
	if err != nil {
		return node.Node{}, fmt.Errorf("find zone: %w", err)
	}
	if z == nil {
		return node.Node{}, fmt.Errorf("zone %q not found", item.Zone)
	}
	item.ZoneUUID = z.UUID
	if err := s.regionConfig.ValidateRegionZone(item.Region, item.Zone); err != nil {
		return node.Node{}, err
	}
	if item.Status == "" {
		item.Status = node.StatusUp
	}
	if !node.IsValidStatus(item.Status) {
		return node.Node{}, fmt.Errorf("status must be up or down")
	}

	existing, err := s.repo.FindByManagementAddress(ctx, item.ManagementAddress)
	if err != nil {
		return node.Node{}, fmt.Errorf("find existing node: %w", err)
	}

	item.UUID = strings.TrimSpace(item.UUID)
	if existing != nil {
		item.UUID = existing.UUID
	}
	if item.UUID == "" {
		item.UUID = uuid.NewString()
	}
	if _, err := uuid.Parse(item.UUID); err != nil {
		return node.Node{}, fmt.Errorf("uuid must be a valid UUID")
	}

	if err := s.repo.Upsert(ctx, item); err != nil {
		return node.Node{}, err
	}
	return item, nil
}

func (s *NodeService) List(ctx context.Context) ([]node.Node, error) {
	return s.repo.List(ctx)
}

func (s *NodeService) GetByUUID(ctx context.Context, uuidValue string) (node.Node, error) {
	uuidValue = strings.TrimSpace(uuidValue)
	if uuidValue == "" {
		return node.Node{}, fmt.Errorf("uuid is required")
	}
	if _, err := uuid.Parse(uuidValue); err != nil {
		return node.Node{}, fmt.Errorf("uuid must be a valid UUID")
	}

	item, err := s.repo.FindByUUID(ctx, uuidValue)
	if err != nil {
		return node.Node{}, err
	}
	if item == nil {
		return node.Node{}, fmt.Errorf("node not found")
	}
	return *item, nil
}

func (s *NodeService) DeleteByUUID(ctx context.Context, uuidValue string) error {
	uuidValue = strings.TrimSpace(uuidValue)
	if uuidValue == "" {
		return fmt.Errorf("uuid is required")
	}
	if _, err := uuid.Parse(uuidValue); err != nil {
		return fmt.Errorf("uuid must be a valid UUID")
	}

	deleted, err := s.repo.DeleteByUUID(ctx, uuidValue)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("node not found")
	}
	return nil
}

func (s *NodeService) SetStatus(ctx context.Context, uuidValue string, status string) (node.Node, error) {
	uuidValue = strings.TrimSpace(uuidValue)
	status = strings.ToLower(strings.TrimSpace(status))

	if uuidValue == "" {
		return node.Node{}, fmt.Errorf("uuid is required")
	}
	if _, err := uuid.Parse(uuidValue); err != nil {
		return node.Node{}, fmt.Errorf("uuid must be a valid UUID")
	}
	if !node.IsValidStatus(status) {
		return node.Node{}, fmt.Errorf("status must be up or down")
	}

	updated, err := s.repo.UpdateStatus(ctx, uuidValue, status)
	if err != nil {
		return node.Node{}, err
	}
	if !updated {
		return node.Node{}, fmt.Errorf("node not found")
	}

	item, err := s.repo.FindByUUID(ctx, uuidValue)
	if err != nil {
		return node.Node{}, err
	}
	if item == nil {
		return node.Node{}, fmt.Errorf("node not found")
	}
	return *item, nil
}

func (s *NodeService) ListRegions(ctx context.Context) ([]node.RegionSummary, error) {
	return s.repo.ListRegions(ctx)
}

func (s *NodeService) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	return s.repo.ListRegionZones(ctx)
}

func (s *NodeService) Report(ctx context.Context, item node.Node) (node.Node, error) {
	item.UUID = strings.TrimSpace(item.UUID)
	item.Hostname = strings.TrimSpace(item.Hostname)
	item.ManagementAddress = strings.TrimSpace(item.ManagementAddress)
	item.InternalIP = strings.TrimSpace(item.InternalIP)
	item.PublicIP = strings.TrimSpace(item.PublicIP)
	item.Region = strings.TrimSpace(strings.ToLower(item.Region))
	item.Zone = strings.TrimSpace(strings.ToUpper(item.Zone))
	item.Status = strings.ToLower(strings.TrimSpace(item.Status))

	if item.UUID == "" {
		return node.Node{}, fmt.Errorf("uuid is required")
	}
	if _, err := uuid.Parse(item.UUID); err != nil {
		return node.Node{}, fmt.Errorf("uuid must be a valid UUID")
	}
	if item.Hostname == "" {
		return node.Node{}, fmt.Errorf("hostname is required")
	}
	if item.ManagementAddress == "" {
		return node.Node{}, fmt.Errorf("management_address is required")
	}
	if item.Region == "" {
		return node.Node{}, fmt.Errorf("region is required")
	}
	if item.Zone == "" {
		return node.Node{}, fmt.Errorf("zone is required")
	}
	r, err := s.regionRepo.FindByName(ctx, item.Region)
	if err != nil {
		return node.Node{}, fmt.Errorf("find region: %w", err)
	}
	if r == nil {
		return node.Node{}, fmt.Errorf("region %q not found", item.Region)
	}
	item.RegionUUID = r.UUID
	z, err := s.zoneRepo.FindByName(ctx, item.Zone)
	if err != nil {
		return node.Node{}, fmt.Errorf("find zone: %w", err)
	}
	if z == nil {
		return node.Node{}, fmt.Errorf("zone %q not found", item.Zone)
	}
	item.ZoneUUID = z.UUID
	if err := s.regionConfig.ValidateRegionZone(item.Region, item.Zone); err != nil {
		return node.Node{}, err
	}
	if item.Status == "" {
		item.Status = node.StatusUp
	}
	if !node.IsValidStatus(item.Status) {
		return node.Node{}, fmt.Errorf("status must be up or down")
	}
	if err := validatePercent("cpu_usage_percent", item.CPUUsagePercent); err != nil {
		return node.Node{}, err
	}
	if err := validatePercent("memory_usage_percent", item.MemoryUsagePercent); err != nil {
		return node.Node{}, err
	}
	if err := validatePercent("disk_usage_percent", item.DiskUsagePercent); err != nil {
		return node.Node{}, err
	}
	if item.SwapTotalGB > 0 {
		if err := validatePercent("swap_usage_percent", item.SwapUsagePercent); err != nil {
			return node.Node{}, err
		}
	}

	if err := s.repo.UpdateHeartbeat(ctx, item); err != nil {
		if err == sql.ErrNoRows {
			return node.Node{}, fmt.Errorf("node not found")
		}
		return node.Node{}, err
	}

	updated, err := s.repo.FindByUUID(ctx, item.UUID)
	if err != nil {
		return node.Node{}, err
	}
	if updated == nil {
		return node.Node{}, fmt.Errorf("node not found")
	}

	if !s.dnsConfig.Enabled || s.dnsProvider == nil || !s.dnsProvider.Enabled() {
		return *updated, nil
	}
	if updated.PublicIP == "" {
		return *updated, nil
	}
	if updated.DNSName == "" {
		updated, err = s.repo.EnsureDNSBinding(ctx, item.UUID, s.dnsConfig.RecordPrefix, s.dnsConfig.Zone)
		if err != nil {
			return node.Node{}, err
		}
		if updated == nil {
			return node.Node{}, fmt.Errorf("node not found")
		}
	}
	if err := s.dnsProvider.EnsureRecord(ctx, platformdns.Record{
		Name:    updated.DNSName,
		Type:    s.dnsConfig.RecordType,
		Content: updated.PublicIP,
		TTL:     s.dnsConfig.TTL,
		Proxied: s.dnsConfig.Proxied,
	}); err != nil {
		return node.Node{}, err
	}
	return *updated, nil
}

func validatePercent(name string, value float64) error {
	if value < 0 || value > 100 {
		return fmt.Errorf("%s must be between 0 and 100", name)
	}
	return nil
}

func (s *NodeService) MarkTimedOutNodesDown(ctx context.Context, timeoutSec int) (int, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return s.repo.MarkTimedOutNodesDown(ctx, timeoutSec)
}
