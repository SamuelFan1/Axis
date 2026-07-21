package node

import (
	"strings"
	"time"
)

const (
	HTTPSProbeTransitionNone      = "none"
	HTTPSProbeTransitionIsolated  = "isolated"
	HTTPSProbeTransitionRecovered = "recovered"
)

type HTTPSProbeState struct {
	ObserverRegion       string    `json:"observer_region"`
	NodeUUID             string    `json:"node_uuid"`
	Isolated             bool      `json:"isolated"`
	ConsecutiveFailures  int       `json:"consecutive_failures"`
	ConsecutiveSuccesses int       `json:"consecutive_successes"`
	LastHTTPStatus       int       `json:"last_http_status"`
	LastError            string    `json:"last_error"`
	LastCheckedAt        time.Time `json:"last_checked_at"`
	LastTransitionAt     time.Time `json:"last_transition_at"`
	NextCheckAt          time.Time `json:"next_check_at"`
	LeaseOwner           string    `json:"lease_owner"`
	LeaseExpiresAt       time.Time `json:"lease_expires_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type HTTPSProbeResult struct {
	Healthy    bool
	HTTPStatus int
	Error      string
	CheckedAt  time.Time
}

func ApplyHTTPSProbeResult(current HTTPSProbeState, result HTTPSProbeResult, failureThreshold, recoveryThreshold int) (HTTPSProbeState, string) {
	if failureThreshold <= 0 {
		failureThreshold = 3
	}
	if recoveryThreshold <= 0 {
		recoveryThreshold = 2
	}
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}

	next := current
	next.LastCheckedAt = result.CheckedAt
	next.LastHTTPStatus = result.HTTPStatus
	next.LastError = strings.TrimSpace(result.Error)
	transition := HTTPSProbeTransitionNone

	if result.Healthy {
		next.ConsecutiveFailures = 0
		if next.ConsecutiveSuccesses < recoveryThreshold {
			next.ConsecutiveSuccesses++
		}
		if next.Isolated && next.ConsecutiveSuccesses >= recoveryThreshold {
			next.Isolated = false
			next.LastTransitionAt = result.CheckedAt
			transition = HTTPSProbeTransitionRecovered
		}
		return next, transition
	}

	next.ConsecutiveSuccesses = 0
	if next.ConsecutiveFailures < failureThreshold {
		next.ConsecutiveFailures++
	}
	if !next.Isolated && next.ConsecutiveFailures >= failureThreshold {
		next.Isolated = true
		next.LastTransitionAt = result.CheckedAt
		transition = HTTPSProbeTransitionIsolated
	}
	return next, transition
}

func ApplyAvailability(item *Node, manualDisabled bool, probe *HTTPSProbeState) {
	if item == nil {
		return
	}
	item.ManualDisabled = manualDisabled
	if probe != nil {
		item.HTTPSProbeIsolated = probe.Isolated
	}
	if manualDisabled {
		item.Status = StatusDown
		item.StatusReason = "manual maintenance"
		return
	}
	if probe != nil && probe.Isolated {
		item.Status = StatusDown
		item.StatusReason = "https probe: " + probeReason(*probe)
	}
}

func probeReason(state HTTPSProbeState) string {
	if strings.TrimSpace(state.LastError) != "" {
		return strings.TrimSpace(state.LastError)
	}
	if state.LastHTTPStatus > 0 {
		return "unexpected HTTP status"
	}
	return "public endpoint unavailable"
}
