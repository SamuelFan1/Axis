package service

import (
	"context"
	"fmt"
	"time"

	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type RegionalStatusSnapshotService struct {
	viewRepo    repository.NodeViewRepository
	localRegion string
}

func NewRegionalStatusSnapshotService(viewRepo repository.NodeViewRepository, localRegion string) *RegionalStatusSnapshotService {
	return &RegionalStatusSnapshotService{
		viewRepo:    viewRepo,
		localRegion: localRegion,
	}
}

func (s *RegionalStatusSnapshotService) Generate(ctx context.Context) (node.RegionalNodeStatusSnapshot, error) {
	items, err := s.viewRepo.List(ctx)
	if err != nil {
		return node.RegionalNodeStatusSnapshot{}, fmt.Errorf("list nodes for regional snapshot: %w", err)
	}
	now := time.Now().UTC()
	out := node.RegionalNodeStatusSnapshot{
		SourceRegion:    s.localRegion,
		SnapshotVersion: fmt.Sprintf("regional-%s-%d", s.localRegion, now.UnixNano()),
		GeneratedAt:     now,
		ObservedAt:      now,
		Nodes:           make([]node.RegionalNodeStatus, 0, len(items)),
	}
	for _, item := range items {
		if item.Region != s.localRegion {
			continue
		}
		out.Nodes = append(out.Nodes, node.RegionalNodeStatus{
			NodeUUID:           item.UUID,
			HomeRegion:         item.Region,
			Status:             item.Status,
			SourceRegion:       s.localRegion,
			InternalIP:         item.InternalIP,
			PublicIP:           item.PublicIP,
			CPUCores:           item.CPUCores,
			CPUUsagePercent:    item.CPUUsagePercent,
			MemoryTotalGB:      item.MemoryTotalGB,
			MemoryUsedGB:       item.MemoryUsedGB,
			MemoryUsagePercent: item.MemoryUsagePercent,
			SwapTotalGB:        item.SwapTotalGB,
			SwapUsedGB:         item.SwapUsedGB,
			SwapUsagePercent:   item.SwapUsagePercent,
			DiskUsagePercent:   item.DiskUsagePercent,
			DiskDetails:        append([]node.DiskDetail(nil), item.DiskDetails...),
			MonitoringSnapshot: append([]byte(nil), item.MonitoringSnapshot...),
			LastSeenAt:         item.LastSeenAt,
			LastReportedAt:     item.LastReportedAt,
			ObservedAt:         now,
		})
	}
	return out, nil
}
