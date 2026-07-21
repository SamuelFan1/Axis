package node

import (
	"encoding/json"
	"time"
)

type RegionalNodeStatusSnapshot struct {
	SourceRegion    string               `json:"source_region"`
	SnapshotVersion string               `json:"snapshot_version"`
	GeneratedAt     time.Time            `json:"generated_at"`
	ObservedAt      time.Time            `json:"observed_at"`
	Nodes           []RegionalNodeStatus `json:"nodes"`
}

type RegionalNodeStatus struct {
	NodeUUID           string          `json:"node_uuid"`
	HomeRegion         string          `json:"home_region"`
	Status             string          `json:"status"`
	StatusReason       string          `json:"status_reason,omitempty"`
	SourceRegion       string          `json:"source_region"`
	InternalIP         string          `json:"internal_ip"`
	PublicIP           string          `json:"public_ip"`
	CPUCores           int             `json:"cpu_cores"`
	CPUUsagePercent    float64         `json:"cpu_usage_percent"`
	MemoryTotalGB      float64         `json:"memory_total_gb"`
	MemoryUsedGB       float64         `json:"memory_used_gb"`
	MemoryUsagePercent float64         `json:"memory_usage_percent"`
	SwapTotalGB        float64         `json:"swap_total_gb"`
	SwapUsedGB         float64         `json:"swap_used_gb"`
	SwapUsagePercent   float64         `json:"swap_usage_percent"`
	DiskUsagePercent   float64         `json:"disk_usage_percent"`
	DiskDetails        []DiskDetail    `json:"disk_details,omitempty"`
	MonitoringSnapshot json.RawMessage `json:"monitoring_snapshot,omitempty"`
	LastSeenAt         time.Time       `json:"last_seen_at"`
	LastReportedAt     time.Time       `json:"last_reported_at"`
	ObservedAt         time.Time       `json:"observed_at"`
}

type AggregatedNodeStatus struct {
	Node
	HomeRegion         string    `json:"home_region"`
	StatusSourceRegion string    `json:"status_source_region"`
	ObservedAt         time.Time `json:"observed_at"`
	Stale              bool      `json:"stale"`
}
