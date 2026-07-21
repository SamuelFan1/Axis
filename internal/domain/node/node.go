package node

import (
	"encoding/json"
	"time"
)

const (
	StatusUp   = "up"
	StatusDown = "down"
)

type DiskDetail struct {
	MountPoint   string  `json:"mount_point"`
	Filesystem   string  `json:"filesystem"`
	TotalGB      float64 `json:"total_gb"`
	UsedGB       float64 `json:"used_gb"`
	UsagePercent float64 `json:"usage_percent"`
}

type NodeIdentity struct {
	UUID              string    `json:"uuid"`
	Hostname          string    `json:"hostname"`
	ManagementAddress string    `json:"management_address"`
	InternalIP        string    `json:"internal_ip"`
	PublicIP          string    `json:"public_ip"`
	DNSLabel          string    `json:"dns_label"`
	DNSName           string    `json:"dns_name"`
	Region            string    `json:"region"`
	RegionUUID        string    `json:"region_uuid,omitempty"`
	Zone              string    `json:"zone"`
	ZoneUUID          string    `json:"zone_uuid,omitempty"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type NodeHealth struct {
	ObserverRegion     string          `json:"observer_region"`
	NodeUUID           string          `json:"node_uuid"`
	Status             string          `json:"status"`
	StatusSource       string          `json:"status_source"`
	CPUCores           int             `json:"cpu_cores"`
	CPUUsagePercent    float64         `json:"cpu_usage_percent"`
	MemoryTotalGB      float64         `json:"memory_total_gb"`
	MemoryUsedGB       float64         `json:"memory_used_gb"`
	MemoryUsagePercent float64         `json:"memory_usage_percent"`
	SwapTotalGB        float64         `json:"swap_total_gb"`
	SwapUsedGB         float64         `json:"swap_used_gb"`
	SwapUsagePercent   float64         `json:"swap_usage_percent"`
	DiskUsagePercent   float64         `json:"disk_usage_percent"`
	DiskDetails        []DiskDetail    `json:"disk_details"`
	MonitoringSnapshot json.RawMessage `json:"monitoring_snapshot,omitempty"`
	LastSeenAt         time.Time       `json:"last_seen_at"`
	LastReportedAt     time.Time       `json:"last_reported_at"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type Node struct {
	UUID               string          `json:"uuid"`
	Hostname           string          `json:"hostname"`
	ManagementAddress  string          `json:"management_address"`
	InternalIP         string          `json:"internal_ip"`
	PublicIP           string          `json:"public_ip"`
	DNSLabel           string          `json:"dns_label"`
	DNSName            string          `json:"dns_name"`
	Region             string          `json:"region"`
	RegionUUID         string          `json:"region_uuid,omitempty"`
	Zone               string          `json:"zone"`
	ZoneUUID           string          `json:"zone_uuid,omitempty"`
	Status             string          `json:"status"`
	StatusReason       string          `json:"status_reason,omitempty"`
	ManualDisabled     bool            `json:"manual_disabled"`
	HTTPSProbeIsolated bool            `json:"https_probe_isolated"`
	CPUCores           int             `json:"cpu_cores"`
	CPUUsagePercent    float64         `json:"cpu_usage_percent"`
	MemoryTotalGB      float64         `json:"memory_total_gb"`
	MemoryUsedGB       float64         `json:"memory_used_gb"`
	MemoryUsagePercent float64         `json:"memory_usage_percent"`
	SwapTotalGB        float64         `json:"swap_total_gb"`
	SwapUsedGB         float64         `json:"swap_used_gb"`
	SwapUsagePercent   float64         `json:"swap_usage_percent"`
	DiskUsagePercent   float64         `json:"disk_usage_percent"`
	DiskDetails        []DiskDetail    `json:"disk_details"`
	MonitoringSnapshot json.RawMessage `json:"monitoring_snapshot,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	LastSeenAt         time.Time       `json:"last_seen_at"`
	LastReportedAt     time.Time       `json:"last_reported_at"`
}

func (i NodeIdentity) Aggregate(health *NodeHealth) Node {
	item := Node{
		UUID:              i.UUID,
		Hostname:          i.Hostname,
		ManagementAddress: i.ManagementAddress,
		InternalIP:        i.InternalIP,
		PublicIP:          i.PublicIP,
		DNSLabel:          i.DNSLabel,
		DNSName:           i.DNSName,
		Region:            i.Region,
		RegionUUID:        i.RegionUUID,
		Zone:              i.Zone,
		ZoneUUID:          i.ZoneUUID,
		CreatedAt:         i.CreatedAt,
		UpdatedAt:         i.UpdatedAt,
	}
	if health == nil {
		return item
	}
	item.Status = health.Status
	item.CPUCores = health.CPUCores
	item.CPUUsagePercent = health.CPUUsagePercent
	item.MemoryTotalGB = health.MemoryTotalGB
	item.MemoryUsedGB = health.MemoryUsedGB
	item.MemoryUsagePercent = health.MemoryUsagePercent
	item.SwapTotalGB = health.SwapTotalGB
	item.SwapUsedGB = health.SwapUsedGB
	item.SwapUsagePercent = health.SwapUsagePercent
	item.DiskUsagePercent = health.DiskUsagePercent
	item.DiskDetails = health.DiskDetails
	item.MonitoringSnapshot = append(item.MonitoringSnapshot[:0], health.MonitoringSnapshot...)
	item.LastSeenAt = health.LastSeenAt
	item.LastReportedAt = health.LastReportedAt
	return item
}

func IsValidStatus(status string) bool {
	switch status {
	case StatusUp, StatusDown:
		return true
	default:
		return false
	}
}
