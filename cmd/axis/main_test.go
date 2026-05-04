package main

import (
	"encoding/json"
	"testing"

	"github.com/SamuelFan1/Axis/internal/domain/node"
)

func serviceListReasonSnapshot(t *testing.T, sources ...map[string]interface{}) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{
		"sources": sources,
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return raw
}

func TestServiceListReasonShowsSourceErrorForUpNode(t *testing.T) {
	item := node.Node{
		Status: node.StatusUp,
		MonitoringSnapshot: serviceListReasonSnapshot(t, map[string]interface{}{
			"name":   "go-sidecar",
			"status": "error",
			"error":  "connection refused",
		}),
	}

	reason := serviceListReason(item)
	if reason != "go-sidecar: connection refused" {
		t.Fatalf("expected go-sidecar reason, got %q", reason)
	}
}

func TestServiceListReasonEmptyForUpNodeWhenSourcesHealthy(t *testing.T) {
	item := node.Node{
		Status: node.StatusUp,
		MonitoringSnapshot: serviceListReasonSnapshot(t, map[string]interface{}{
			"name":   "go-sidecar",
			"status": "ok",
		}),
	}

	if reason := serviceListReason(item); reason != "" {
		t.Fatalf("expected empty reason, got %q", reason)
	}
}

func TestServiceListReasonStillReportsDownNode(t *testing.T) {
	item := node.Node{Status: node.StatusDown}

	if reason := serviceListReason(item); reason != "reported down" {
		t.Fatalf("expected reported down reason, got %q", reason)
	}
}
