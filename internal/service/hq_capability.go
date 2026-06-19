package service

import (
	"encoding/json"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
)

type HQCapability struct {
	Detected       bool
	ServiceHealthy bool
	SessionLoaded  bool
	Ready          bool
	Version        string
}

type monitoringEnvelope struct {
	Sources []monitoringSource `json:"sources"`
}

type monitoringSource struct {
	Name    string                 `json:"name"`
	Status  string                 `json:"status"`
	Summary map[string]interface{} `json:"summary"`
}

func HQCapabilityForNode(item node.Node, cfg config.HQConfig) HQCapability {
	sourceName := strings.TrimSpace(cfg.MonitoringSource)
	if sourceName == "" {
		sourceName = "yt-dlp-hq"
	}
	var envelope monitoringEnvelope
	if len(item.MonitoringSnapshot) == 0 {
		return HQCapability{}
	}
	if err := json.Unmarshal(item.MonitoringSnapshot, &envelope); err != nil {
		return HQCapability{}
	}
	for _, source := range envelope.Sources {
		if !strings.EqualFold(strings.TrimSpace(source.Name), sourceName) {
			continue
		}
		capability := HQCapability{
			Detected:       boolFromSummary(source.Summary, "detected"),
			ServiceHealthy: boolFromSummary(source.Summary, "service_healthy"),
			SessionLoaded:  boolFromSummary(source.Summary, "session_loaded"),
			Ready:          boolFromSummary(source.Summary, "ready"),
			Version:        stringFromSummary(source.Summary, "version"),
		}
		if !strings.EqualFold(strings.TrimSpace(source.Status), "ok") {
			capability.Ready = false
		}
		return capability
	}
	return HQCapability{}
}

func HQServiceHostFromBinding(binding dnsbinding.Binding, cfg config.HQConfig) (string, bool) {
	label := strings.TrimSpace(binding.DNSLabel)
	sourcePrefix := strings.TrimSpace(cfg.DerivedFromRecordPrefix)
	if sourcePrefix == "" {
		sourcePrefix = "dl-"
	}
	if label == "" || !strings.HasPrefix(label, sourcePrefix) {
		return "", false
	}
	suffix := strings.TrimPrefix(label, sourcePrefix)
	if suffix == "" || !isDNSNumericSuffix(suffix) {
		return "", false
	}
	targetPrefix := strings.TrimSpace(cfg.DNSRecordPrefix)
	if targetPrefix == "" {
		targetPrefix = "n"
	}
	zone := strings.Trim(strings.TrimSpace(cfg.DNSZone), ".")
	if zone == "" {
		return "", false
	}
	return platformdns.BuildDNSName(targetPrefix+suffix, zone), true
}

func boolFromSummary(summary map[string]interface{}, key string) bool {
	if summary == nil {
		return false
	}
	switch value := summary[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func stringFromSummary(summary map[string]interface{}, key string) string {
	if summary == nil {
		return ""
	}
	if value, ok := summary[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func isDNSNumericSuffix(value string) bool {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
