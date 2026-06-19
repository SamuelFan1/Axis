package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type HQDNSSyncResult struct {
	Expected       int
	Ensured        int
	Skipped        int
	MissingBinding int
}

type HQDNSService struct {
	nodeViewRepo repository.NodeViewRepository
	bindingRepo  repository.DNSBindingRepository
	dnsProvider  platformdns.Provider
	cfg          config.HQConfig
}

func NewHQDNSService(nodeViewRepo repository.NodeViewRepository, bindingRepo repository.DNSBindingRepository, dnsProvider platformdns.Provider, cfg config.HQConfig) *HQDNSService {
	return &HQDNSService{
		nodeViewRepo: nodeViewRepo,
		bindingRepo:  bindingRepo,
		dnsProvider:  dnsProvider,
		cfg:          cfg,
	}
}

func (s *HQDNSService) Sync(ctx context.Context) (HQDNSSyncResult, error) {
	if s.nodeViewRepo == nil {
		return HQDNSSyncResult{}, fmt.Errorf("node view repository is not configured")
	}
	if s.bindingRepo == nil {
		return HQDNSSyncResult{}, fmt.Errorf("dns binding repository is not configured")
	}
	if s.dnsProvider == nil || !s.dnsProvider.Enabled() {
		return HQDNSSyncResult{}, fmt.Errorf("dns provider is not configured")
	}

	nodes, err := s.nodeViewRepo.List(ctx)
	if err != nil {
		return HQDNSSyncResult{}, fmt.Errorf("list nodes for hq dns sync: %w", err)
	}
	bindings, err := s.bindingRepo.List(ctx)
	if err != nil {
		return HQDNSSyncResult{}, fmt.Errorf("list dns bindings for hq dns sync: %w", err)
	}
	bindingByNode := make(map[string]dnsbinding.Binding, len(bindings))
	for _, binding := range bindings {
		bindingByNode[strings.TrimSpace(binding.NodeUUID)] = binding
	}

	result := HQDNSSyncResult{}
	for _, item := range nodes {
		capability := HQCapabilityForNode(item, s.cfg)
		if !capability.Detected {
			result.Skipped++
			continue
		}
		publicIP := strings.TrimSpace(item.PublicIP)
		if publicIP == "" {
			result.Skipped++
			continue
		}
		binding, ok := bindingByNode[strings.TrimSpace(item.UUID)]
		if !ok {
			result.MissingBinding++
			continue
		}
		host, ok := HQServiceHostFromBinding(binding, s.cfg)
		if !ok {
			result.Skipped++
			continue
		}
		result.Expected++
		if err := s.dnsProvider.EnsureRecord(ctx, platformdns.Record{
			Name:    host,
			Type:    s.cfg.DNSRecordType,
			Content: publicIP,
			TTL:     s.cfg.DNSTTL,
			Proxied: s.cfg.DNSProxied,
		}); err != nil {
			return result, fmt.Errorf("ensure hq dns record %s: %w", host, err)
		}
		result.Ensured++
	}
	return result, nil
}
