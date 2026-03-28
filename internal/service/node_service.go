package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/routing"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	"github.com/SamuelFan1/Axis/internal/platform/workeradmin"
	"github.com/SamuelFan1/Axis/internal/repository"
	"github.com/google/uuid"
)

type NodeService struct {
	identityRepo   repository.NodeIdentityRepository
	healthRepo     repository.NodeHealthRepository
	localViewRepo  repository.NodeViewRepository
	globalViewRepo repository.AggregatedNodeRepository
	regionRepo     repository.RegionRepository
	zoneRepo       repository.ZoneRepository
	dnsProvider    platformdns.Provider
	dnsBindingRepo repository.DNSBindingRepository
	dnsConfig      config.DNSConfig
	regionConfig   config.RegionConfig
	workerAdmin    workeradmin.Client
}

type NodeStatusResult struct {
	Node                node.Node
	ExternalMaintenance bool
	WorkerSynced        bool
}

func NewNodeService(identityRepo repository.NodeIdentityRepository, healthRepo repository.NodeHealthRepository, localViewRepo repository.NodeViewRepository, globalViewRepo repository.AggregatedNodeRepository, regionRepo repository.RegionRepository, zoneRepo repository.ZoneRepository, dnsProvider platformdns.Provider, dnsBindingRepo repository.DNSBindingRepository, dnsConfig config.DNSConfig, regionConfig config.RegionConfig, workerAdmin workeradmin.Client) *NodeService {
	return &NodeService{
		identityRepo:   identityRepo,
		healthRepo:     healthRepo,
		localViewRepo:  localViewRepo,
		globalViewRepo: globalViewRepo,
		regionRepo:     regionRepo,
		zoneRepo:       zoneRepo,
		dnsProvider:    dnsProvider,
		dnsBindingRepo: dnsBindingRepo,
		dnsConfig:      dnsConfig,
		regionConfig:   regionConfig,
		workerAdmin:    workerAdmin,
	}
}

func (s *NodeService) EnsureSchema(ctx context.Context) error {
	if err := s.identityRepo.EnsureSchema(ctx); err != nil {
		return err
	}
	if err := s.healthRepo.EnsureSchema(ctx); err != nil {
		return err
	}
	if s.dnsConfig.Enabled && s.dnsBindingRepo != nil {
		if err := s.dnsBindingRepo.EnsureSchema(ctx); err != nil {
			return fmt.Errorf("ensure dns binding schema: %w", err)
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
	z, err := s.zoneRepo.FindByRegionUUIDAndName(ctx, item.RegionUUID, item.Zone)
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

	existing, err := s.identityRepo.FindActiveByManagementAddress(ctx, item.ManagementAddress)
	if err != nil {
		return node.Node{}, fmt.Errorf("find existing node: %w", err)
	}

	item.UUID = strings.TrimSpace(item.UUID)
	if item.UUID == "" {
		item.UUID = uuid.NewString()
	}
	if _, err := uuid.Parse(item.UUID); err != nil {
		return node.Node{}, fmt.Errorf("uuid must be a valid UUID")
	}
	if existing != nil && existing.UUID != item.UUID {
		if err := s.identityRepo.ArchiveAndDeleteByManagementAddress(ctx, item.ManagementAddress, item.UUID, "replaced_by_new_uuid"); err != nil {
			return node.Node{}, fmt.Errorf("archive existing node: %w", err)
		}
	}

	if err := s.identityRepo.UpsertIdentity(ctx, node.NodeIdentity{
		UUID:              item.UUID,
		Hostname:          item.Hostname,
		ManagementAddress: item.ManagementAddress,
		InternalIP:        item.InternalIP,
		PublicIP:          item.PublicIP,
		DNSLabel:          item.DNSLabel,
		DNSName:           item.DNSName,
		Region:            item.Region,
		RegionUUID:        item.RegionUUID,
		Zone:              item.Zone,
		ZoneUUID:          item.ZoneUUID,
		Status:            item.Status,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
	}); err != nil {
		return node.Node{}, err
	}
	return item, nil
}

func (s *NodeService) List(ctx context.Context) ([]node.Node, error) {
	return s.readViewRepo().List(ctx)
}

func (s *NodeService) GetByUUID(ctx context.Context, uuidValue string) (node.Node, error) {
	uuidValue = strings.TrimSpace(uuidValue)
	if uuidValue == "" {
		return node.Node{}, fmt.Errorf("uuid is required")
	}
	if _, err := uuid.Parse(uuidValue); err != nil {
		return node.Node{}, fmt.Errorf("uuid must be a valid UUID")
	}

	item, err := s.readViewRepo().FindByUUID(ctx, uuidValue)
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

	deleted, err := s.identityRepo.DeleteByUUID(ctx, uuidValue)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("node not found")
	}
	return nil
}

func (s *NodeService) SetStatus(ctx context.Context, uuidValue string, status string) (NodeStatusResult, error) {
	uuidValue = strings.TrimSpace(uuidValue)
	status = strings.ToLower(strings.TrimSpace(status))

	if uuidValue == "" {
		return NodeStatusResult{}, fmt.Errorf("uuid is required")
	}
	if _, err := uuid.Parse(uuidValue); err != nil {
		return NodeStatusResult{}, fmt.Errorf("uuid must be a valid UUID")
	}
	if !node.IsValidStatus(status) {
		return NodeStatusResult{}, fmt.Errorf("status must be up or down")
	}

	item, err := s.readViewRepo().FindByUUID(ctx, uuidValue)
	if err != nil {
		return NodeStatusResult{}, err
	}
	if item == nil {
		return NodeStatusResult{}, fmt.Errorf("node not found")
	}
	if s.workerAdmin == nil || !s.workerAdmin.Enabled() {
		return NodeStatusResult{}, fmt.Errorf("worker admin is not configured")
	}

	originLabel := routing.OriginLabelForHostname(item.Hostname)
	if status == node.StatusDown {
		if err := s.workerAdmin.DisableNode(ctx, originLabel); err != nil {
			return NodeStatusResult{}, err
		}
		return NodeStatusResult{
			Node:                *item,
			ExternalMaintenance: true,
			WorkerSynced:        true,
		}, nil
	}
	if err := s.workerAdmin.EnableNode(ctx, originLabel); err != nil {
		return NodeStatusResult{}, err
	}
	return NodeStatusResult{
		Node:                *item,
		ExternalMaintenance: false,
		WorkerSynced:        true,
	}, nil
}

func (s *NodeService) ListRegions(ctx context.Context) ([]node.RegionSummary, error) {
	return s.readViewRepo().ListRegions(ctx)
}

func (s *NodeService) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	return s.readViewRepo().ListRegionZones(ctx)
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

	items, err := s.readViewRepo().List(ctx)
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
	z, err := s.zoneRepo.FindByRegionUUIDAndName(ctx, item.RegionUUID, item.Zone)
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
	item.Status = s.applyMonitoringHealthPolicy(item.Status, item.MonitoringSnapshot)

	identity, err := s.identityRepo.FindIdentityByUUID(ctx, item.UUID)
	if err != nil {
		return node.Node{}, err
	}
	if identity == nil {
		return node.Node{}, fmt.Errorf("node not found")
	}
	identity.Hostname = item.Hostname
	identity.ManagementAddress = item.ManagementAddress
	identity.InternalIP = item.InternalIP
	identity.PublicIP = item.PublicIP
	if err := s.identityRepo.UpsertIdentity(ctx, *identity); err != nil {
		return node.Node{}, err
	}

	if err := s.healthRepo.UpsertHealth(ctx, node.NodeHealth{
		ObserverRegion:     s.observerRegion(item.Region),
		NodeUUID:           item.UUID,
		Status:             item.Status,
		StatusSource:       "self_report",
		CPUCores:           item.CPUCores,
		CPUUsagePercent:    item.CPUUsagePercent,
		MemoryTotalGB:      item.MemoryTotalGB,
		MemoryUsedGB:       item.MemoryUsedGB,
		MemoryUsagePercent: item.MemoryUsagePercent,
		SwapTotalGB:        item.SwapTotalGB,
		SwapUsedGB:         item.SwapUsedGB,
		SwapUsagePercent:   item.SwapUsagePercent,
		DiskUsagePercent:   item.DiskUsagePercent,
		DiskDetails:        item.DiskDetails,
		MonitoringSnapshot: item.MonitoringSnapshot,
	}); err != nil {
		if err == sql.ErrNoRows {
			return node.Node{}, fmt.Errorf("node not found")
		}
		return node.Node{}, err
	}

	updated, err := s.localViewRepo.FindByUUID(ctx, item.UUID)
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
	if s.dnsBindingRepo == nil {
		return node.Node{}, fmt.Errorf("dns binding repository is not configured")
	}

	return s.ensureCentralDNSBinding(ctx, updated)
}

func (s *NodeService) GetMonitoringSnapshot(ctx context.Context, uuidValue string) (json.RawMessage, error) {
	if hasRepositoryValue(s.globalViewRepo) {
		return s.globalViewRepo.GetMonitoringSnapshot(ctx, uuidValue)
	}
	snapshot, err := s.healthRepo.GetMonitoringSnapshot(ctx, uuidValue)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *NodeService) readViewRepo() repository.NodeViewRepository {
	if hasRepositoryValue(s.globalViewRepo) {
		return s.globalViewRepo
	}
	return s.localViewRepo
}

func hasRepositoryValue(repo interface{}) bool {
	if repo == nil {
		return false
	}
	value := reflect.ValueOf(repo)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func (s *NodeService) ensureCentralDNSBinding(ctx context.Context, item *node.Node) (node.Node, error) {
	binding, err := s.dnsBindingRepo.GetByNodeUUID(ctx, item.UUID)
	if err != nil {
		return node.Node{}, fmt.Errorf("get dns binding: %w", err)
	}
	if binding == nil {
		binding, err = s.dnsBindingRepo.AllocateForNode(ctx, item.UUID, s.dnsConfig.Zone, s.dnsConfig.RecordPrefix)
		if err != nil {
			return node.Node{}, fmt.Errorf("allocate dns binding: %w", err)
		}
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

	if err := s.dnsBindingRepo.UpdateLastPublicIP(ctx, item.UUID, item.PublicIP); err != nil && err != sql.ErrNoRows {
		return node.Node{}, fmt.Errorf("update dns binding last public ip: %w", err)
	}
	if err := s.saveDNSBinding(ctx, item.UUID, binding.DNSLabel, binding.DNSName); err != nil {
		return node.Node{}, err
	}

	item.DNSLabel = binding.DNSLabel
	item.DNSName = binding.DNSName
	return *item, nil
}

func (s *NodeService) saveDNSBinding(ctx context.Context, uuid string, label string, name string) error {
	if err := s.identityRepo.SaveDNSBinding(ctx, uuid, label, name); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("node not found")
		}
		return fmt.Errorf("save dns binding: %w", err)
	}
	return nil
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

func (s *NodeService) applyMonitoringHealthPolicy(status string, snapshot json.RawMessage) string {
	if !s.dnsConfig.RequireCFTunnelHealth {
		return status
	}
	if strings.ToLower(strings.TrimSpace(status)) == node.StatusDown {
		return node.StatusDown
	}
	if tunnelSourceHealthy(snapshot, s.dnsConfig.CFTunnelSourceName) {
		return status
	}
	return node.StatusDown
}

func tunnelSourceHealthy(snapshot json.RawMessage, sourceName string) bool {
	sourceName = strings.TrimSpace(sourceName)
	if sourceName == "" || len(snapshot) == 0 {
		return false
	}

	var payload struct {
		Sources []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(snapshot, &payload); err != nil {
		return false
	}
	for _, source := range payload.Sources {
		if strings.TrimSpace(source.Name) != sourceName {
			continue
		}
		return strings.EqualFold(strings.TrimSpace(source.Status), "ok")
	}
	return false
}

func (s *NodeService) MarkTimedOutNodesDown(ctx context.Context, timeoutSec int) (int, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return s.healthRepo.MarkTimedOutNodesDown(ctx, s.regionConfig.LocalRegion, timeoutSec)
}

func (s *NodeService) observerRegion(fallback string) string {
	regionValue := strings.TrimSpace(strings.ToLower(s.regionConfig.LocalRegion))
	if regionValue != "" {
		return regionValue
	}
	return strings.TrimSpace(strings.ToLower(fallback))
}
