package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/observation"
	"github.com/SamuelFan1/Axis/internal/domain/routing"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type RoutingSnapshotService struct {
	observationRepo repository.ObservationRepository
	snapshotRepo    repository.RoutingSnapshotRepository
	nodeViewRepo    repository.NodeViewRepository
	regionRepo      repository.RegionRepository
	zoneRepo        repository.ZoneRepository
	cfg             config.RoutingConfig
}

func NewRoutingSnapshotService(
	observationRepo repository.ObservationRepository,
	snapshotRepo repository.RoutingSnapshotRepository,
	nodeViewRepo repository.NodeViewRepository,
	regionRepo repository.RegionRepository,
	zoneRepo repository.ZoneRepository,
	cfg config.RoutingConfig,
) *RoutingSnapshotService {
	return &RoutingSnapshotService{
		observationRepo: observationRepo,
		snapshotRepo:    snapshotRepo,
		nodeViewRepo:    nodeViewRepo,
		regionRepo:      regionRepo,
		zoneRepo:        zoneRepo,
		cfg:             cfg,
	}
}

func (s *RoutingSnapshotService) EnsureSchema(ctx context.Context) error {
	return s.snapshotRepo.EnsureSchema(ctx)
}

func (s *RoutingSnapshotService) GenerateAndStore(ctx context.Context) (routing.Manifest, []routing.Bundle, error) {
	manifest, bundles, err := s.Generate(ctx)
	if err != nil {
		return routing.Manifest{}, nil, err
	}
	if err := s.snapshotRepo.SaveManifest(ctx, manifest); err != nil {
		return routing.Manifest{}, nil, err
	}
	if err := s.snapshotRepo.SaveBundles(ctx, bundles); err != nil {
		return routing.Manifest{}, nil, err
	}
	return manifest, bundles, nil
}

func (s *RoutingSnapshotService) Generate(ctx context.Context) (routing.Manifest, []routing.Bundle, error) {
	nodes, err := s.nodeViewRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list nodes for routing snapshot: %w", err)
	}
	observations, err := s.observationRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list routing observations: %w", err)
	}
	regionItems, err := s.regionRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list regions for routing snapshot: %w", err)
	}
	zones, err := s.zoneRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list zones for routing snapshot: %w", err)
	}

	validRegions := make(map[string]struct{}, len(regionItems))
	for _, item := range regionItems {
		name := strings.TrimSpace(strings.ToLower(item.Name))
		if name == "" {
			continue
		}
		validRegions[name] = struct{}{}
	}
	validZones := make(map[string]struct{}, len(zones))
	for _, item := range zones {
		rn := strings.TrimSpace(strings.ToLower(item.RegionName))
		zn := strings.TrimSpace(strings.ToUpper(item.Name))
		if rn == "" || zn == "" {
			continue
		}
		validZones[rn+"\x00"+zn] = struct{}{}
	}

	upNodes := make(map[string]node.Node)
	for _, item := range nodes {
		if strings.ToLower(strings.TrimSpace(item.Status)) != node.StatusUp {
			continue
		}
		regionKey := strings.TrimSpace(strings.ToLower(item.Region))
		zoneKey := strings.TrimSpace(strings.ToUpper(item.Zone))
		if _, ok := validRegions[regionKey]; !ok {
			continue
		}
		if _, ok := validZones[regionKey+"\x00"+zoneKey]; !ok {
			continue
		}
		upNodes[item.UUID] = item
	}

	now := time.Now().UTC()
	version := fmt.Sprintf("v%d", now.UnixNano())
	expiresAt := now.Add(time.Duration(s.cfg.SnapshotTTLSeconds) * time.Second)
	topN := s.cfg.TopN
	if topN <= 0 {
		topN = 3
	}

	zoneCandidates := make(map[string][]routing.Candidate)
	regionCandidates := make(map[string][]routing.Candidate)
	globalCandidates := make([]routing.Candidate, 0, len(upNodes))

	for _, item := range upNodes {
		candidate := fallbackCandidate(item)
		zoneCandidates[item.Zone] = append(zoneCandidates[item.Zone], candidate)
		regionCandidates[item.Region] = append(regionCandidates[item.Region], candidate)
		globalCandidates = append(globalCandidates, candidate)
	}
	for zone := range zoneCandidates {
		sortFallbackCandidates(zoneCandidates[zone])
		zoneCandidates[zone] = trimCandidates(zoneCandidates[zone], topN)
	}
	for region := range regionCandidates {
		sortFallbackCandidates(regionCandidates[region])
		regionCandidates[region] = trimCandidates(regionCandidates[region], topN)
	}
	sortFallbackCandidates(globalCandidates)
	globalCandidates = trimCandidates(globalCandidates, topN)

	perColo := make(map[string][]routing.Candidate)
	for _, item := range observations {
		nodeItem, ok := upNodes[item.TargetNodeUUID]
		if !ok {
			continue
		}
		if item.SourceColo == "" {
			continue
		}
		perColo[item.SourceColo] = append(perColo[item.SourceColo], observedCandidate(nodeItem, item))
	}

	bundlesByRegion := make(map[string]map[string][]routing.Candidate)
	for sourceColo, candidates := range perColo {
		sortObservedCandidates(candidates)
		for _, candidate := range trimCandidates(candidates, topN) {
			regionName := candidate.Region
			if bundlesByRegion[regionName] == nil {
				bundlesByRegion[regionName] = make(map[string][]routing.Candidate)
			}
			bundlesByRegion[regionName][sourceColo] = append(bundlesByRegion[regionName][sourceColo], candidate)
		}
	}

	bundleRegions := make([]string, 0, len(bundlesByRegion))
	for regionName := range bundlesByRegion {
		bundleRegions = append(bundleRegions, regionName)
	}
	sort.Strings(bundleRegions)

	bundles := make([]routing.Bundle, 0, len(bundleRegions))
	bundleRefs := make([]routing.BundleRef, 0, len(bundleRegions))
	for _, regionName := range bundleRegions {
		entries := bundlesByRegion[regionName]
		for sourceColo := range entries {
			sortObservedCandidates(entries[sourceColo])
			entries[sourceColo] = trimCandidates(entries[sourceColo], topN)
		}
		key := routing.BundleKVKey(regionName)
		bundle := routing.Bundle{
			Version:     version,
			Region:      regionName,
			Key:         key,
			GeneratedAt: now,
			ExpiresAt:   expiresAt,
			Entries:     entries,
		}
		bundles = append(bundles, bundle)
		bundleRefs = append(bundleRefs, routing.BundleRef{
			Region: regionName,
			Key:    key,
		})
	}

	manifest := routing.Manifest{
		Version:          version,
		GeneratedAt:      now,
		ExpiresAt:        expiresAt,
		TopN:             topN,
		Bundles:          bundleRefs,
		ZoneCandidates:   zoneCandidates,
		RegionCandidates: regionCandidates,
		GlobalCandidates: globalCandidates,
	}
	return manifest, bundles, nil
}

func (s *RoutingSnapshotService) GetLatest(ctx context.Context) (*routing.Manifest, []routing.Bundle, error) {
	manifest, err := s.snapshotRepo.GetLatestManifest(ctx)
	if err != nil {
		return nil, nil, err
	}
	if manifest == nil {
		return nil, nil, nil
	}
	bundles, err := s.snapshotRepo.ListBundlesByVersion(ctx, manifest.Version)
	if err != nil {
		return nil, nil, err
	}
	return manifest, bundles, nil
}

func (s *RoutingSnapshotService) GetByVersion(ctx context.Context, version string) (*routing.Manifest, []routing.Bundle, error) {
	manifest, err := s.snapshotRepo.GetManifestByVersion(ctx, version)
	if err != nil {
		return nil, nil, err
	}
	if manifest == nil {
		return nil, nil, nil
	}
	bundles, err := s.snapshotRepo.ListBundlesByVersion(ctx, version)
	if err != nil {
		return nil, nil, err
	}
	return manifest, bundles, nil
}

func fallbackCandidate(item node.Node) routing.Candidate {
	return routing.Candidate{
		NodeUUID:    item.UUID,
		Hostname:    item.Hostname,
		OriginLabel: routing.OriginLabelForHostname(item.Hostname),
		Region:      item.Region,
		Zone:        item.Zone,
		Score:       resourceScore(item),
	}
}

func observedCandidate(item node.Node, obs observation.Aggregate) routing.Candidate {
	avgLatencyMs := obs.AverageLatencyMs()
	errorRate := obs.ErrorRate()
	latencyScore := avgLatencyMs
	if latencyScore <= 0 {
		latencyScore = 100000
	}
	score := latencyScore + errorRate*1000 + resourceScore(item)*2
	return routing.Candidate{
		NodeUUID:       item.UUID,
		Hostname:       item.Hostname,
		OriginLabel:    routing.OriginLabelForHostname(item.Hostname),
		Region:         item.Region,
		Zone:           item.Zone,
		Score:          score,
		AvgLatencyMs:   avgLatencyMs,
		ErrorRate:      errorRate,
		SampleCount:    obs.SampleCount,
		LastObservedAt: obs.LastObservedAt,
	}
}

func sortFallbackCandidates(items []routing.Candidate) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			if items[i].Region == items[j].Region {
				if items[i].Zone == items[j].Zone {
					return items[i].Hostname < items[j].Hostname
				}
				return items[i].Zone < items[j].Zone
			}
			return items[i].Region < items[j].Region
		}
		return items[i].Score < items[j].Score
	})
}

func sortObservedCandidates(items []routing.Candidate) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			if items[i].AvgLatencyMs == items[j].AvgLatencyMs {
				return items[i].Hostname < items[j].Hostname
			}
			return items[i].AvgLatencyMs < items[j].AvgLatencyMs
		}
		return items[i].Score < items[j].Score
	})
}

func trimCandidates(items []routing.Candidate, limit int) []routing.Candidate {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]routing.Candidate(nil), items[:limit]...)
}

func resourceScore(item node.Node) float64 {
	return item.DiskUsagePercent*0.5 + item.CPUUsagePercent*0.3 + item.MemoryUsagePercent*0.2
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
