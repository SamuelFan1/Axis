package service

import (
	"testing"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	"github.com/SamuelFan1/Axis/internal/domain/node"
)

func TestHQCapabilityForNodeRequiresReadySource(t *testing.T) {
	item := node.Node{
		MonitoringSnapshot: []byte(`{"sources":[{"name":"yt-dlp-hq","status":"ok","summary":{"detected":true,"service_healthy":true,"session_loaded":true,"ready":true,"version":"1.2.0"}}]}`),
	}
	capability := HQCapabilityForNode(item, config.HQConfig{MonitoringSource: "yt-dlp-hq"})
	if !capability.Detected || !capability.ServiceHealthy || !capability.SessionLoaded || !capability.Ready || capability.Version != "1.2.0" {
		t.Fatalf("unexpected capability: %+v", capability)
	}
}

func TestHQCapabilityForNodeStatusErrorClearsReady(t *testing.T) {
	item := node.Node{
		MonitoringSnapshot: []byte(`{"sources":[{"name":"yt-dlp-hq","status":"error","summary":{"detected":true,"service_healthy":true,"session_loaded":false,"ready":false}}]}`),
	}
	capability := HQCapabilityForNode(item, config.HQConfig{MonitoringSource: "yt-dlp-hq"})
	if !capability.Detected || capability.Ready {
		t.Fatalf("expected detected but not ready, got %+v", capability)
	}
}

func TestHQServiceHostFromBindingPreservesDLNumericSuffix(t *testing.T) {
	host, ok := HQServiceHostFromBinding(dnsbinding.Binding{DNSLabel: "dl-001"}, config.HQConfig{
		DNSZone:                 "aiplexlink.com",
		DNSRecordPrefix:         "n",
		DerivedFromRecordPrefix: "dl-",
	})
	if !ok || host != "n001.aiplexlink.com" {
		t.Fatalf("expected n001.aiplexlink.com, got %q ok=%v", host, ok)
	}
}
