package service

import (
	"context"
	"testing"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	"github.com/SamuelFan1/Axis/internal/domain/node"
)

func TestHQDNSSyncCreatesDerivedDNSOnlyRecord(t *testing.T) {
	provider := &stubDNSProvider{enabled: true}
	service := NewHQDNSService(
		stubAuxiliaryNodeViewRepo{items: []node.Node{
			{
				UUID:               "node-184",
				PublicIP:           "109.123.232.41",
				MonitoringSnapshot: hqReadyMonitoringSnapshot(),
			},
		}},
		&stubDNSBindingRepository{bindings: map[string]dnsbinding.Binding{
			"node-184": {
				NodeUUID:     "node-184",
				DNSLabel:     "dl-184",
				DNSName:      "dl-184.nuxdisk.com",
				Zone:         "nuxdisk.com",
				RecordPrefix: "dl-",
			},
		}},
		provider,
		testHQConfig(),
	)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if result.Expected != 1 || result.Ensured != 1 {
		t.Fatalf("expected one ensured record, got %+v", result)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("expected one DNS call, got %d", len(provider.calls))
	}
	record := provider.calls[0]
	if record.Name != "n184.aiplexlink.com" || record.Content != "109.123.232.41" || record.Type != "A" || record.Proxied {
		t.Fatalf("unexpected DNS record: %+v", record)
	}
}

func TestHQDNSSyncSkipsNodesWithoutHQDeployment(t *testing.T) {
	provider := &stubDNSProvider{enabled: true}
	service := NewHQDNSService(
		stubAuxiliaryNodeViewRepo{items: []node.Node{
			{
				UUID:               "node-001",
				PublicIP:           "203.0.113.1",
				MonitoringSnapshot: []byte(`{"sources":[{"name":"go-sidecar","status":"ok","summary":{}}]}`),
			},
		}},
		&stubDNSBindingRepository{bindings: map[string]dnsbinding.Binding{
			"node-001": {
				NodeUUID:     "node-001",
				DNSLabel:     "dl-001",
				DNSName:      "dl-001.nuxdisk.com",
				Zone:         "nuxdisk.com",
				RecordPrefix: "dl-",
			},
		}},
		provider,
		testHQConfig(),
	)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if result.Expected != 0 || result.Ensured != 0 || result.Skipped != 1 {
		t.Fatalf("expected one skipped node, got %+v", result)
	}
	if len(provider.calls) != 0 {
		t.Fatalf("expected no DNS calls, got %+v", provider.calls)
	}
}

func hqReadyMonitoringSnapshot() []byte {
	return []byte(`{"sources":[{"name":"yt-dlp-hq","status":"ok","summary":{"detected":true,"service_healthy":true,"session_loaded":true,"ready":true}}]}`)
}

func hqSessionMissingMonitoringSnapshot() []byte {
	return []byte(`{"sources":[{"name":"yt-dlp-hq","status":"error","summary":{"detected":true,"service_healthy":true,"session_loaded":false,"ready":false}}]}`)
}

func testHQConfig() config.HQConfig {
	return config.HQConfig{
		DNSZone:                 "aiplexlink.com",
		DNSRecordPrefix:         "n",
		DNSRecordType:           "A",
		DNSTTL:                  1,
		DNSProxied:              false,
		DNSSyncIntervalSec:      60,
		MonitoringSource:        "yt-dlp-hq",
		RoutingManifestKey:      "hq:routing:manifest",
		RoutingBundlePrefix:     "hq:routing:bundle:",
		DerivedFromRecordPrefix: "dl-",
	}
}
