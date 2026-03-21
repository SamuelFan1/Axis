package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type AggregatedNodeService struct {
	baseViewRepo   repository.NodeViewRepository
	snapshotRepo   repository.RegionalNodeStatusSnapshotRepository
	aggregatedRepo repository.AggregatedNodeRepository
	staleAfter     time.Duration
}

func NewAggregatedNodeService(
	baseViewRepo repository.NodeViewRepository,
	snapshotRepo repository.RegionalNodeStatusSnapshotRepository,
	aggregatedRepo repository.AggregatedNodeRepository,
	staleAfterSec int,
) *AggregatedNodeService {
	if staleAfterSec <= 0 {
		staleAfterSec = 90
	}
	return &AggregatedNodeService{
		baseViewRepo:   baseViewRepo,
		snapshotRepo:   snapshotRepo,
		aggregatedRepo: aggregatedRepo,
		staleAfter:     time.Duration(staleAfterSec) * time.Second,
	}
}

func (s *AggregatedNodeService) Rebuild(ctx context.Context) (int, error) {
	baseItems, err := s.baseViewRepo.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list base nodes for aggregation: %w", err)
	}
	snapshots, err := s.snapshotRepo.ListLatest(ctx)
	if err != nil {
		return 0, fmt.Errorf("list regional snapshots for aggregation: %w", err)
	}
	byRegionAndNode := make(map[string]node.RegionalNodeStatus, len(snapshots))
	for _, item := range snapshots {
		key := strings.TrimSpace(strings.ToLower(item.SourceRegion)) + "\x00" + item.NodeUUID
		byRegionAndNode[key] = item
	}

	now := time.Now().UTC()
	aggregated := make([]node.AggregatedNodeStatus, 0, len(baseItems))
	for _, base := range baseItems {
		homeRegion := strings.TrimSpace(strings.ToLower(base.Region))
		key := homeRegion + "\x00" + base.UUID
		snapshot, ok := byRegionAndNode[key]
		item := node.AggregatedNodeStatus{
			Node:               base,
			HomeRegion:         homeRegion,
			StatusSourceRegion: homeRegion,
			ObservedAt:         now,
			Stale:              true,
		}
		item.Status = node.StatusDown
		if ok {
			item.Status = normalizedStatus(snapshot.Status)
			item.StatusSourceRegion = strings.TrimSpace(strings.ToLower(snapshot.SourceRegion))
			item.ObservedAt = snapshot.ObservedAt
			item.Stale = isSnapshotStale(snapshot, now, s.staleAfter)
			item.InternalIP = snapshot.InternalIP
			item.PublicIP = snapshot.PublicIP
			item.CPUCores = snapshot.CPUCores
			item.CPUUsagePercent = snapshot.CPUUsagePercent
			item.MemoryTotalGB = snapshot.MemoryTotalGB
			item.MemoryUsedGB = snapshot.MemoryUsedGB
			item.MemoryUsagePercent = snapshot.MemoryUsagePercent
			item.SwapTotalGB = snapshot.SwapTotalGB
			item.SwapUsedGB = snapshot.SwapUsedGB
			item.SwapUsagePercent = snapshot.SwapUsagePercent
			item.DiskUsagePercent = snapshot.DiskUsagePercent
			item.DiskDetails = append([]node.DiskDetail(nil), snapshot.DiskDetails...)
			item.MonitoringSnapshot = append(item.MonitoringSnapshot[:0], snapshot.MonitoringSnapshot...)
			item.LastSeenAt = snapshot.LastSeenAt
			item.LastReportedAt = snapshot.LastReportedAt
			if item.Stale {
				item.Status = node.StatusDown
			}
		}
		aggregated = append(aggregated, item)
	}
	if err := s.aggregatedRepo.ReplaceAll(ctx, aggregated); err != nil {
		return 0, fmt.Errorf("replace aggregated node status: %w", err)
	}
	return len(aggregated), nil
}

func normalizedStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	if status == node.StatusUp {
		return node.StatusUp
	}
	return node.StatusDown
}

func isSnapshotStale(item node.RegionalNodeStatus, now time.Time, threshold time.Duration) bool {
	if threshold <= 0 {
		return false
	}
	reference := item.ObservedAt
	if !item.LastReportedAt.IsZero() {
		reference = item.LastReportedAt
	}
	if reference.IsZero() {
		return true
	}
	return now.Sub(reference) > threshold
}
