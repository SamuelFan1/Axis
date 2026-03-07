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
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastSeenAt        time.Time
}

func IsValidStatus(status string) bool {
	switch status {
	case StatusUp, StatusDown:
		return true
	default:
		return false
	}
}
