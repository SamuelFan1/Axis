package dnsbinding

import "time"

type Binding struct {
	NodeUUID     string    `json:"node_uuid"`
	DNSLabel     string    `json:"dns_label"`
	DNSName      string    `json:"dns_name"`
	Zone         string    `json:"zone"`
	RecordPrefix string    `json:"record_prefix"`
	Sequence     int       `json:"sequence"`
	LastPublicIP string    `json:"last_public_ip"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
