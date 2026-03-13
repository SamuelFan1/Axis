package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

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
	bindingStore platformdns.BindingStore
	resolver     platformdns.Resolver
	dnsConfig    config.DNSConfig
	regionConfig config.RegionConfig
}

func NewNodeService(repo repository.NodeRepository, regionRepo repository.RegionRepository, zoneRepo repository.ZoneRepository, dnsProvider platformdns.Provider, bindingStore platformdns.BindingStore, resolver platformdns.Resolver, dnsConfig config.DNSConfig, regionConfig config.RegionConfig) *NodeService {
	return &NodeService{
		repo:         repo,
		regionRepo:   regionRepo,
		zoneRepo:     zoneRepo,
		dnsProvider:  dnsProvider,
		bindingStore: bindingStore,
		resolver:     resolver,
		dnsConfig:    dnsConfig,
		regionConfig: regionConfig,
	}
}

func (s *NodeService) EnsureSchema(ctx context.Context) error {
	return s.repo.EnsureSchema(ctx)
}

func (s *NodeService) SyncDNSBindingsFromLocal(ctx context.Context) error {
	if !s.dnsConfig.Enabled || s.bindingStore == nil {
		return nil
	}

	bindings, err := s.bindingStore.List()
	if err != nil {
		return fmt.Errorf("list local dns bindings: %w", err)
	}

	for _, binding := range bindings {
		item, err := s.repo.FindByUUID(ctx, binding.NodeUUID)
		if err != nil {
			return fmt.Errorf("find node for local dns binding %s: %w", binding.NodeUUID, err)
		}
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.DNSLabel) == binding.DNSLabel && strings.TrimSpace(item.DNSName) == binding.DNSName {
			continue
		}
		if err := s.repo.SaveDNSBinding(ctx, binding.NodeUUID, binding.DNSLabel, binding.DNSName); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return fmt.Errorf("sync local dns binding %s: %w", binding.NodeUUID, err)
		}
	}

	return nil
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

func (s *NodeService) AssignByRegionZone(ctx context.Context, region string, zone string) (node.Node, error) {
	region = strings.TrimSpace(strings.ToLower(region))
	zone = strings.TrimSpace(strings.ToUpper(zone))

	if region == "" {
		return node.Node{}, fmt.Errorf("region is required")
	}
	if zone == "" {
		return node.Node{}, fmt.Errorf("zone is required")
	}
	if err := s.regionConfig.ValidateRegionZone(region, zone); err != nil {
		return node.Node{}, err
	}

	items, err := s.repo.List(ctx)
	if err != nil {
		return node.Node{}, err
	}

	regionCandidates := filterUpNodesByRegion(items, region)
	if len(regionCandidates) == 0 {
		return node.Node{}, fmt.Errorf("node not found")
	}

	zoneCandidates := filterNodesByZone(regionCandidates, zone)
	if len(zoneCandidates) > 0 {
		selected, ok := pickLowestScoreNode(zoneCandidates)
		if !ok {
			return node.Node{}, fmt.Errorf("node not found")
		}
		return selected, nil
	}

	selected, ok := pickLowestScoreNode(regionCandidates)
	if !ok {
		return node.Node{}, fmt.Errorf("node not found")
	}
	return selected, nil
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

	currentBinding, err := s.bindingStore.Load(updated.UUID)
	if err != nil {
		return node.Node{}, fmt.Errorf("load local dns binding: %w", err)
	}

	if currentBinding == nil {
		return s.assignNewDNSBinding(ctx, updated)
	}

	resolvedIPs, err := s.resolver.LookupA(ctx, currentBinding.DNSName)
	if err != nil {
		return node.Node{}, fmt.Errorf("lookup dns binding %s: %w", currentBinding.DNSName, err)
	}

	currentBinding.LastPublicIP = updated.PublicIP
	currentBinding.UpdatedAt = time.Now().UTC()
	if containsString(resolvedIPs, updated.PublicIP) {
		if err := s.bindingStore.Save(*currentBinding); err != nil {
			return node.Node{}, fmt.Errorf("save local dns binding: %w", err)
		}
		if strings.TrimSpace(updated.DNSLabel) != currentBinding.DNSLabel || strings.TrimSpace(updated.DNSName) != currentBinding.DNSName {
			if err := s.saveDNSBinding(ctx, updated.UUID, currentBinding.DNSLabel, currentBinding.DNSName); err != nil {
				return node.Node{}, err
			}
			updated.DNSLabel = currentBinding.DNSLabel
			updated.DNSName = currentBinding.DNSName
		}
		return *updated, nil
	}

	return s.assignNewDNSBinding(ctx, updated)
}

func (s *NodeService) GetMonitoringSnapshot(ctx context.Context, uuidValue string) (json.RawMessage, error) {
	item, err := s.GetByUUID(ctx, uuidValue)
	if err != nil {
		return nil, err
	}
	return item.MonitoringSnapshot, nil
}

func (s *NodeService) assignNewDNSBinding(ctx context.Context, item *node.Node) (node.Node, error) {
	binding, err := s.nextDNSBinding(item.UUID, item.PublicIP)
	if err != nil {
		return node.Node{}, err
	}

	if err := s.dnsProvider.EnsureRecord(ctx, platformdns.Record{
		Name:    binding.DNSName,
		Type:    s.dnsConfig.RecordType,
		Content: item.PublicIP,
		TTL:     s.dnsConfig.TTL,
		Proxied: s.dnsConfig.Proxied,
	}); err != nil {
		return node.Node{}, err
	}

	if err := s.bindingStore.Save(binding); err != nil {
		return node.Node{}, fmt.Errorf("save local dns binding: %w", err)
	}
	if err := s.saveDNSBinding(ctx, item.UUID, binding.DNSLabel, binding.DNSName); err != nil {
		return node.Node{}, err
	}

	item.DNSLabel = binding.DNSLabel
	item.DNSName = binding.DNSName
	return *item, nil
}

func (s *NodeService) nextDNSBinding(nodeUUID string, publicIP string) (platformdns.Binding, error) {
	sequence, err := s.bindingStore.ReserveNextSequence(s.dnsConfig.RecordPrefix)
	if err != nil {
		return platformdns.Binding{}, fmt.Errorf("reserve dns sequence: %w", err)
	}

	label := platformdns.BuildDNSLabel(s.dnsConfig.RecordPrefix, sequence)
	return platformdns.Binding{
		NodeUUID:     strings.TrimSpace(nodeUUID),
		DNSLabel:     label,
		DNSName:      platformdns.BuildDNSName(label, s.dnsConfig.Zone),
		LastPublicIP: strings.TrimSpace(publicIP),
		UpdatedAt:    time.Now().UTC(),
	}, nil
}

func (s *NodeService) saveDNSBinding(ctx context.Context, uuid string, label string, name string) error {
	if err := s.repo.SaveDNSBinding(ctx, uuid, label, name); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("node not found")
		}
		return fmt.Errorf("save dns binding: %w", err)
	}
	return nil
}

func containsString(items []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, item := range items {
		if strings.TrimSpace(item) == expected {
			return true
		}
	}
	return false
}

func weightedScore(item node.Node) float64 {
	return item.DiskUsagePercent*0.5 + item.CPUUsagePercent*0.3 + item.MemoryUsagePercent*0.2
}

func filterUpNodesByRegion(items []node.Node, region string) []node.Node {
	candidates := make([]node.Node, 0, len(items))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.Status)) != node.StatusUp {
			continue
		}
		if strings.TrimSpace(strings.ToLower(item.Region)) != region {
			continue
		}
		candidates = append(candidates, item)
	}
	return candidates
}

func filterNodesByZone(items []node.Node, zone string) []node.Node {
	candidates := make([]node.Node, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(strings.ToUpper(item.Zone)) != zone {
			continue
		}
		candidates = append(candidates, item)
	}
	return candidates
}

func pickLowestScoreNode(items []node.Node) (node.Node, bool) {
	if len(items) == 0 {
		return node.Node{}, false
	}

	bestScore := weightedScore(items[0])
	bestItems := []node.Node{items[0]}
	for _, item := range items[1:] {
		score := weightedScore(item)
		switch {
		case score < bestScore:
			bestScore = score
			bestItems = []node.Node{item}
		case score == bestScore:
			bestItems = append(bestItems, item)
		}
	}

	return bestItems[rand.IntN(len(bestItems))], true
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
	return s.repo.MarkTimedOutNodesDown(ctx, s.regionConfig.LocalRegion, timeoutSec)
}
