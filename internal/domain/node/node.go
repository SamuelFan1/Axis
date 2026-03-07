package node

import "time"

const (
	StatusUp   = "up"
	StatusDown = "down"
)

type Node struct {
	UUID              string
	Hostname          string
	ManagementAddress string
	Region            string
	Status            string
	CPUUsagePercent   float64
	MemoryUsagePercent float64
	DiskUsagePercent  float64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastSeenAt        time.Time
	LastReportedAt    time.Time
}

func IsValidStatus(status string) bool {
	switch status {
	case StatusUp, StatusDown:
		return true
	default:
		return false
	}
}
