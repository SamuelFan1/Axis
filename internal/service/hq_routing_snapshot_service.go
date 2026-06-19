package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/observation"
	"github.com/SamuelFan1/Axis/internal/domain/routing"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type HQRoutingSnapshotService struct {
	observationRepo repository.ObservationRepository
	nodeViewRepo    repository.NodeViewRepository
	regionRepo      repository.RegionRepository
	zoneRepo        repository.ZoneRepository
	bindingRepo     repository.DNSBindingRepository
	routingCfg      config.RoutingConfig
	hqCfg           config.HQConfig
}

type hqNodeCandidate struct {
	node        node.Node
	serviceHost string
}

func NewHQRoutingSnapshotService(
	observationRepo repository.ObservationRepository,
	nodeViewRepo repository.NodeViewRepository,
	regionRepo repository.RegionRepository,
	zoneRepo repository.ZoneRepository,
	bindingRepo repository.DNSBindingRepository,
	routingCfg config.RoutingConfig,
	hqCfg config.HQConfig,
) *HQRoutingSnapshotService {
	return &HQRoutingSnapshotService{
		observationRepo: observationRepo,
		nodeViewRepo:    nodeViewRepo,
		regionRepo:      regionRepo,
		zoneRepo:        zoneRepo,
		bindingRepo:     bindingRepo,
		routingCfg:      routingCfg,
		hqCfg:           hqCfg,
	}
}

func (s *HQRoutingSnapshotService) Generate(ctx context.Context) (routing.Manifest, []routing.Bundle, error) {
	nodes, err := s.nodeViewRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list nodes for hq routing snapshot: %w", err)
	}
	observations, err := s.observationRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list routing observations for hq snapshot: %w", err)
	}
	bindings, err := s.bindingRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list dns bindings for hq routing snapshot: %w", err)
	}
	regionItems, err := s.regionRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list regions for hq routing snapshot: %w", err)
	}
	zones, err := s.zoneRepo.List(ctx)
	if err != nil {
		return routing.Manifest{}, nil, fmt.Errorf("list zones for hq routing snapshot: %w", err)
	}

	validRegions := make(map[string]struct{}, len(regionItems))
	for _, item := range regionItems {
		name := strings.TrimSpace(strings.ToLower(item.Name))
		if name != "" {
			validRegions[name] = struct{}{}
		}
	}
	validZones := make(map[string]struct{}, len(zones))
	for _, item := range zones {
		rn := strings.TrimSpace(strings.ToLower(item.RegionName))
		zn := strings.TrimSpace(strings.ToUpper(item.Name))
		if rn != "" && zn != "" {
			validZones[rn+"\x00"+zn] = struct{}{}
		}
	}

	bindingByNode := make(map[string]dnsbinding.Binding, len(bindings))
	for _, binding := range bindings {
		bindingByNode[strings.TrimSpace(binding.NodeUUID)] = binding
	}

	upNodes := make(map[string]hqNodeCandidate)
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
		capability := HQCapabilityForNode(item, s.hqCfg)
		if !capability.Detected || !capability.ServiceHealthy || !capability.SessionLoaded || !capability.Ready {
			continue
		}
		binding, ok := bindingByNode[strings.TrimSpace(item.UUID)]
		if !ok {
			continue
		}
		serviceHost, ok := HQServiceHostFromBinding(binding, s.hqCfg)
		if !ok {
			continue
		}
		upNodes[item.UUID] = hqNodeCandidate{
			node:        item,
			serviceHost: serviceHost,
		}
	}

	now := time.Now().UTC()
	version := fmt.Sprintf("hq-v%d", now.UnixNano())
	expiresAt := now.Add(time.Duration(s.routingCfg.SnapshotTTLSeconds) * time.Second)
	topN := s.routingCfg.TopN
	if topN <= 0 {
		topN = 3
	}

	zoneCandidates := make(map[string][]routing.Candidate)
	regionCandidates := make(map[string][]routing.Candidate)
	globalCandidates := make([]routing.Candidate, 0, len(upNodes))

	for _, item := range upNodes {
		candidate := hqFallbackCandidate(item)
		zoneCandidates[item.node.Zone] = append(zoneCandidates[item.node.Zone], candidate)
		regionCandidates[item.node.Region] = append(regionCandidates[item.node.Region], candidate)
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
		if !ok || item.SourceColo == "" {
			continue
		}
		perColo[item.SourceColo] = append(perColo[item.SourceColo], hqObservedCandidate(nodeItem, item))
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
		key := routing.BundleKVKeyWithPrefix(s.hqCfg.RoutingBundlePrefix, regionName)
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
		Key:              strings.TrimSpace(s.hqCfg.RoutingManifestKey),
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

func hqFallbackCandidate(item hqNodeCandidate) routing.Candidate {
	candidate := fallbackCandidate(item.node)
	candidate.ServiceHost = item.serviceHost
	return candidate
}

func hqObservedCandidate(item hqNodeCandidate, obs observation.Aggregate) routing.Candidate {
	candidate := observedCandidate(item.node, obs)
	candidate.ServiceHost = item.serviceHost
	return candidate
}
