package node

import (
	"testing"
	"time"
)

func TestHTTPSProbeTransitionsAfterConfiguredThresholds(t *testing.T) {
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	state := HTTPSProbeState{ObserverRegion: "asia", NodeUUID: "node-1"}
	for i := 0; i < 2; i++ {
		var transition string
		state, transition = ApplyHTTPSProbeResult(state, HTTPSProbeResult{CheckedAt: now.Add(time.Duration(i) * time.Second), Error: "timeout"}, 3, 2)
		if transition != HTTPSProbeTransitionNone || state.Isolated {
			t.Fatalf("unexpected early isolation: state=%+v transition=%s", state, transition)
		}
	}
	state, transition := ApplyHTTPSProbeResult(state, HTTPSProbeResult{CheckedAt: now.Add(2 * time.Second), Error: "timeout"}, 3, 2)
	if transition != HTTPSProbeTransitionIsolated || !state.Isolated || state.ConsecutiveFailures != 3 {
		t.Fatalf("expected isolation on third failure: state=%+v transition=%s", state, transition)
	}

	state, transition = ApplyHTTPSProbeResult(state, HTTPSProbeResult{Healthy: true, HTTPStatus: 200, CheckedAt: now.Add(3 * time.Second)}, 3, 2)
	if transition != HTTPSProbeTransitionNone || !state.Isolated || state.ConsecutiveSuccesses != 1 {
		t.Fatalf("expected one recovery success to remain isolated: state=%+v transition=%s", state, transition)
	}
	state, transition = ApplyHTTPSProbeResult(state, HTTPSProbeResult{Healthy: true, HTTPStatus: 200, CheckedAt: now.Add(4 * time.Second)}, 3, 2)
	if transition != HTTPSProbeTransitionRecovered || state.Isolated || state.ConsecutiveSuccesses != 2 {
		t.Fatalf("expected recovery on second success: state=%+v transition=%s", state, transition)
	}
}

func TestManualMaintenanceOverridesRecoveredHTTPSProbe(t *testing.T) {
	item := Node{Status: StatusUp}
	probe := HTTPSProbeState{Isolated: false, ConsecutiveSuccesses: 2}
	ApplyAvailability(&item, true, &probe)
	if item.Status != StatusDown || item.StatusReason != "manual maintenance" || !item.ManualDisabled {
		t.Fatalf("manual maintenance must remain authoritative: %+v", item)
	}
}
