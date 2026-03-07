package node

import "time"

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
	UUID               string
	Hostname           string
	ManagementAddress  string
	InternalIP         string
	PublicIP           string
	Region             string
	Status             string
	CPUCores           int
	CPUUsagePercent    float64
	MemoryTotalGB      float64
	MemoryUsedGB       float64
	MemoryUsagePercent float64
	SwapTotalGB        float64
	SwapUsedGB         float64
	SwapUsagePercent   float64
	DiskUsagePercent   float64
	DiskDetails        []DiskDetail
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastSeenAt         time.Time
	LastReportedAt     time.Time
}

func IsValidStatus(status string) bool {
	switch status {
	case StatusUp, StatusDown:
		return true
	default:
		return false
	}
}
