package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type MainDNSSyncResult struct {
	Expected         int
	Existing         int
	Created          int
	UpdatedOrEnsured int
	Skipped          int
	MissingBinding   int
}

type MainDNSSyncService struct {
	nodeViewRepo  repository.NodeViewRepository
	bindingRepo   repository.DNSBindingRepository
	recordManager platformdns.RecordManager
	dnsConfig     config.DNSConfig
	zone          string
}

type mainDNSExpectation struct {
	name   string
	record platformdns.Record
}

func NewMainDNSSyncService(nodeViewRepo repository.NodeViewRepository, bindingRepo repository.DNSBindingRepository, recordManager platformdns.RecordManager, dnsConfig config.DNSConfig) *MainDNSSyncService {
	return &MainDNSSyncService{
		nodeViewRepo:  nodeViewRepo,
		bindingRepo:   bindingRepo,
		recordManager: recordManager,
		dnsConfig:     dnsConfig,
		zone:          strings.Trim(strings.TrimSpace(dnsConfig.Zone), "."),
	}
}

func (s *MainDNSSyncService) Sync(ctx context.Context) (MainDNSSyncResult, error) {
	if s.nodeViewRepo == nil {
		return MainDNSSyncResult{}, fmt.Errorf("node view repository is not configured")
	}
	if s.bindingRepo == nil {
		return MainDNSSyncResult{}, fmt.Errorf("dns binding repository is not configured")
	}
	if s.recordManager == nil {
		return MainDNSSyncResult{}, fmt.Errorf("dns record manager is not configured")
	}
	if s.zone == "" {
		return MainDNSSyncResult{}, fmt.Errorf("dns zone is required")
	}

	nodes, err := s.nodeViewRepo.List(ctx)
	if err != nil {
		return MainDNSSyncResult{}, fmt.Errorf("list nodes for main dns sync: %w", err)
	}
	bindings, err := s.bindingRepo.List(ctx)
	if err != nil {
		return MainDNSSyncResult{}, fmt.Errorf("list dns bindings for main dns sync: %w", err)
	}
	expected, missingBinding := s.expectedRecords(nodes, bindings)

	existingRecords, err := s.recordManager.ListRecords(ctx, s.dnsConfig.RecordType)
	if err != nil {
		return MainDNSSyncResult{}, err
	}

	result := MainDNSSyncResult{
		Expected:       len(expected),
		Existing:       len(existingRecords),
		MissingBinding: missingBinding,
	}
	existingByName := make(map[string][]platformdns.ManagedRecord, len(existingRecords))
	for _, record := range existingRecords {
		if !s.recordBelongsToMainNamespace(record.Name) || !strings.EqualFold(strings.TrimSpace(record.Type), s.dnsConfig.RecordType) {
			result.Skipped++
			continue
		}
		name := normalizeDNSName(record.Name)
		if _, ok := expected[name]; !ok {
			result.Skipped++
			continue
		}
		existingByName[name] = append(existingByName[name], record)
	}

	for name, expectation := range expected {
		matches := existingByName[name]
		if len(matches) == 0 {
			if err := s.recordManager.CreateRecord(ctx, expectation.record); err != nil {
				return result, fmt.Errorf("create main dns record %s: %w", expectation.name, err)
			}
			result.Created++
			continue
		}
		for _, match := range matches {
			if match.Content == expectation.record.Content && match.TTL == expectation.record.TTL && match.Proxied == expectation.record.Proxied {
				continue
			}
			if err := s.recordManager.UpdateRecord(ctx, match.ID, expectation.record); err != nil {
				return result, fmt.Errorf("update main dns record %s: %w", expectation.name, err)
			}
			result.UpdatedOrEnsured++
		}
	}

	return result, nil
}

func (s *MainDNSSyncService) expectedRecords(nodes []node.Node, bindings []dnsbinding.Binding) (map[string]mainDNSExpectation, int) {
	bindingByNode := make(map[string]dnsbinding.Binding, len(bindings))
	for _, binding := range bindings {
		if !s.bindingBelongsToMainNamespace(binding) {
			continue
		}
		bindingByNode[strings.TrimSpace(binding.NodeUUID)] = binding
	}

	expected := make(map[string]mainDNSExpectation)
	missingBinding := 0
	for _, item := range nodes {
		if !strings.EqualFold(strings.TrimSpace(item.Status), node.StatusUp) {
			continue
		}
		publicIP := strings.TrimSpace(item.PublicIP)
		if publicIP == "" {
			continue
		}
		binding, ok := bindingByNode[strings.TrimSpace(item.UUID)]
		if !ok {
			missingBinding++
			continue
		}
		name := normalizeDNSName(binding.DNSName)
		record := platformdns.Record{
			Name:    binding.DNSName,
			Type:    s.dnsConfig.RecordType,
			Content: publicIP,
			TTL:     s.dnsConfig.TTL,
			Proxied: s.dnsConfig.Proxied,
		}
		expected[name] = mainDNSExpectation{
			name:   binding.DNSName,
			record: record,
		}
	}
	return expected, missingBinding
}

func (s *MainDNSSyncService) bindingBelongsToMainNamespace(binding dnsbinding.Binding) bool {
	if strings.Trim(strings.TrimSpace(binding.Zone), ".") != s.zone {
		return false
	}
	if strings.TrimSpace(binding.RecordPrefix) != strings.TrimSpace(s.dnsConfig.RecordPrefix) {
		return false
	}
	return s.recordBelongsToMainNamespace(binding.DNSName)
}

func (s *MainDNSSyncService) recordBelongsToMainNamespace(name string) bool {
	normalizedName := normalizeDNSName(name)
	normalizedZone := strings.ToLower(s.zone)
	if normalizedName == normalizedZone {
		return false
	}
	if !strings.HasSuffix(normalizedName, "."+normalizedZone) {
		return false
	}
	label := strings.TrimSuffix(normalizedName, "."+normalizedZone)
	return strings.HasPrefix(label, strings.ToLower(strings.TrimSpace(s.dnsConfig.RecordPrefix)))
}

func normalizeDNSName(name string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(name)), ".")
}
