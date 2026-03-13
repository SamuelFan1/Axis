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

type Node struct {
	UUID               string       `json:"uuid"`
	Hostname           string       `json:"hostname"`
	ManagementAddress  string       `json:"management_address"`
	InternalIP         string       `json:"internal_ip"`
	PublicIP           string       `json:"public_ip"`
	DNSLabel           string       `json:"dns_label"`
	DNSName            string       `json:"dns_name"`
	Region             string       `json:"region"`
	RegionUUID         string       `json:"region_uuid,omitempty"`
	Zone               string       `json:"zone"`
	ZoneUUID           string       `json:"zone_uuid,omitempty"`
	Status             string       `json:"status"`
	CPUCores           int          `json:"cpu_cores"`
	CPUUsagePercent    float64      `json:"cpu_usage_percent"`
	MemoryTotalGB      float64      `json:"memory_total_gb"`
	MemoryUsedGB       float64      `json:"memory_used_gb"`
	MemoryUsagePercent float64      `json:"memory_usage_percent"`
	SwapTotalGB        float64      `json:"swap_total_gb"`
	SwapUsedGB         float64      `json:"swap_used_gb"`
	SwapUsagePercent   float64      `json:"swap_usage_percent"`
	DiskUsagePercent   float64      `json:"disk_usage_percent"`
	DiskDetails        []DiskDetail `json:"disk_details"`
	MonitoringSnapshot json.RawMessage `json:"monitoring_snapshot,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
	LastSeenAt         time.Time    `json:"last_seen_at"`
	LastReportedAt     time.Time    `json:"last_reported_at"`
}

func IsValidStatus(status string) bool {
	switch status {
	case StatusUp, StatusDown:
		return true
	default:
		return false
	}
}
