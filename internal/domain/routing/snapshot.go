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
	ServiceHost    string    `json:"service_host,omitempty"`
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
	Key              string                 `json:"key,omitempty"`
	Version          string                 `json:"version"`
	GeneratedAt      time.Time              `json:"generated_at"`
	ExpiresAt        time.Time              `json:"expires_at"`
	TopN             int                    `json:"top_n"`
	Bundles          []BundleRef            `json:"bundles"`
	ZoneCandidates   map[string][]Candidate `json:"zone_candidates"`
	RegionCandidates map[string][]Candidate `json:"region_candidates"`
	GlobalCandidates []Candidate            `json:"global_candidates"`
}

// BundleKVKey returns a fixed (non-versioned) KV key for a routing bundle.
// Using a fixed key ensures each publish overwrites the previous bundle
// and prevents unbounded KV key accumulation.
func BundleKVKey(region string) string {
	return fmt.Sprintf("routing:bundle:%s", region)
}

func BundleKVKeyWithPrefix(prefix string, region string) string {
	if prefix == "" {
		return BundleKVKey(region)
	}
	return fmt.Sprintf("%s%s", prefix, region)
}
