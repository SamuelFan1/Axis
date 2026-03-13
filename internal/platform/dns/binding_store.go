package dns

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Binding struct {
	NodeUUID     string    `json:"node_uuid"`
	DNSLabel     string    `json:"dns_label"`
	DNSName      string    `json:"dns_name"`
	LastPublicIP string    `json:"last_public_ip"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BindingStore interface {
	Load(nodeUUID string) (*Binding, error)
	Save(binding Binding) error
	List() ([]Binding, error)
	ReserveNextSequence(prefix string) (int, error)
}

func BuildDNSLabel(prefix string, sequence int) string {
	return fmt.Sprintf("%s%03d", strings.TrimSpace(prefix), sequence)
}

func ParseDNSSequence(prefix, label string) (int, bool) {
	prefix = strings.TrimSpace(prefix)
	label = strings.TrimSpace(label)
	if prefix == "" || label == "" || !strings.HasPrefix(label, prefix) {
		return 0, false
	}

	suffix := strings.TrimPrefix(label, prefix)
	if suffix == "" {
		return 0, false
	}
	for _, ch := range suffix {
		if ch < '0' || ch > '9' {
			return 0, false
		}
	}

	value, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, false
	}
	return value, true
}

func BuildDNSName(label, zone string) string {
	trimmedZone := strings.Trim(strings.TrimSpace(zone), ".")
	return strings.TrimSpace(label) + "." + trimmedZone
}
