package routing

import (
	"fmt"
	"time"
)

const ManifestKVKey = "routing:manifest"

type Candidate struct {
	NodeUUID       string    `json:"node_uuid"`
	Hostname       string    `json:"hostname"`
	OriginLabel    string    `json:"origin_label"`
	Region         string    `json:"region"`
	Zone           string    `json:"zone"`
	Score          float64   `json:"score"`
	AvgLatencyMs   float64   `json:"avg_latency_ms"`
	ErrorRate      float64   `json:"error_rate"`
	SampleCount    int64     `json:"sample_count"`
	LastObservedAt time.Time `json:"last_observed_at,omitempty"`
}

type BundleRef struct {
	Region string `json:"region"`
	Key    string `json:"key"`
}

type Bundle struct {
	Version     string                 `json:"version"`
	Region      string                 `json:"region"`
	Key         string                 `json:"key"`
	GeneratedAt time.Time              `json:"generated_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
	Entries     map[string][]Candidate `json:"entries"`
}

type Manifest struct {
	Version          string                 `json:"version"`
	GeneratedAt      time.Time              `json:"generated_at"`
	ExpiresAt        time.Time              `json:"expires_at"`
	TopN             int                    `json:"top_n"`
	Bundles          []BundleRef            `json:"bundles"`
	ZoneCandidates   map[string][]Candidate `json:"zone_candidates"`
	RegionCandidates map[string][]Candidate `json:"region_candidates"`
	GlobalCandidates []Candidate            `json:"global_candidates"`
}

func BundleKVKey(version, region string) string {
	return fmt.Sprintf("routing:snapshot:%s:%s", version, region)
}
