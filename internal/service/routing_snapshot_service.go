package service

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/observation"
	"github.com/SamuelFan1/Axis/internal/domain/routing"
	"github.com/SamuelFan1/Axis/internal/repository"
)

var invalidOriginLabelChars = regexp.MustCompile(`[^a-z0-9-]+`)

type RoutingSnapshotService struct {
	observationRepo repository.ObservationRepository
	snapshotRepo    repository.RoutingSnapshotRepository
	nodeRepo        repository.NodeRepository
	cfg             config.RoutingConfig
}

func NewRoutingSnapshotService(
	observationRepo repository.ObservationRepository,
	snapshotRepo repository.RoutingSnapshotRepository,
	nodeRepo repository.NodeRepository,
	cfg config.RoutingConfig,
) *RoutingSnapshotService {
	return &RoutingSnapshotService{
		observationRepo: observationRepo,
		snapshotRepo:    snapshotRepo,
		nodeRepo:        nodeRepo,
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
	nodes, err := s.nodeRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list nodes for routing snapshot: %w", err)
	}
	observations, err := s.observationRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list routing observations: %w", err)
	}

	upNodes := make(map[string]node.Node)
	for _, item := range nodes {
		if strings.ToLower(strings.TrimSpace(item.Status)) != node.StatusUp {
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

	regions := make([]string, 0, len(bundlesByRegion))
	for regionName := range bundlesByRegion {
		regions = append(regions, regionName)
	}
	sort.Strings(regions)

	bundles := make([]routing.Bundle, 0, len(regions))
	bundleRefs := make([]routing.BundleRef, 0, len(regions))
	for _, regionName := range regions {
		entries := bundlesByRegion[regionName]
		for sourceColo := range entries {
			sortObservedCandidates(entries[sourceColo])
			entries[sourceColo] = trimCandidates(entries[sourceColo], topN)
		}
		key := routing.BundleKVKey(version, regionName)
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
		OriginLabel: originLabelForHostname(item.Hostname),
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
		OriginLabel:    originLabelForHostname(item.Hostname),
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

func originLabelForHostname(hostname string) string {
	label := strings.ToLower(strings.TrimSpace(hostname))
	label = invalidOriginLabelChars.ReplaceAllString(label, "-")
	label = strings.Trim(label, "-")
	label = strings.ReplaceAll(label, "--", "-")
	for strings.Contains(label, "--") {
		label = strings.ReplaceAll(label, "--", "-")
	}
	if label == "" {
		label = "node"
	}
	maxLabelLen := 63 - len("api-origin-")
	if len(label) > maxLabelLen {
		label = label[:maxLabelLen]
	}
	label = strings.Trim(label, "-")
	if label == "" {
		label = "node"
	}
	return "api-origin-" + label
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
