package observation

import "time"

type RecordInput struct {
	SourceColo     string    `json:"source_colo"`
	TargetNodeUUID string    `json:"target_node_uuid"`
	LatencyMs      float64   `json:"latency_ms"`
	Success        bool      `json:"success"`
	ObservedAt     time.Time `json:"observed_at"`
	SampleCount    int64     `json:"sample_count,omitempty"`
}

type Aggregate struct {
	SourceColo          string    `json:"source_colo"`
	TargetNodeUUID      string    `json:"target_node_uuid"`
	SuccessLatencySumMs float64   `json:"success_latency_sum_ms"`
	SuccessCount        int64     `json:"success_count"`
	ErrorCount          int64     `json:"error_count"`
	SampleCount         int64     `json:"sample_count"`
	LastObservedAt      time.Time `json:"last_observed_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (a Aggregate) AverageLatencyMs() float64 {
	if a.SuccessCount <= 0 {
		return 0
	}
	return a.SuccessLatencySumMs / float64(a.SuccessCount)
}

func (a Aggregate) ErrorRate() float64 {
	if a.SampleCount <= 0 {
		return 0
	}
	return float64(a.ErrorCount) / float64(a.SampleCount)
}
