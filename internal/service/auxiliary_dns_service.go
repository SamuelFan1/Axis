package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	"github.com/SamuelFan1/Axis/internal/repository"
)

type AuxiliaryDNSSyncResult struct {
	Expected         int
	Existing         int
	Created          int
	UpdatedOrEnsured int
	Deleted          int
	Skipped          int
}

type AuxiliaryDNSService struct {
	nodeViewRepo  repository.NodeViewRepository
	recordManager platformdns.RecordManager
	dnsConfig     config.DNSConfig
	zone          string
}

type auxiliaryDNSExpectation struct {
	name    string
	content string
	record  platformdns.Record
}

func NewAuxiliaryDNSService(nodeViewRepo repository.NodeViewRepository, recordManager platformdns.RecordManager, dnsConfig config.DNSConfig) *AuxiliaryDNSService {
	return &AuxiliaryDNSService{
		nodeViewRepo:  nodeViewRepo,
		recordManager: recordManager,
		dnsConfig:     dnsConfig,
		zone:          strings.Trim(strings.TrimSpace(dnsConfig.AuxiliaryZone), "."),
	}
}

func (s *AuxiliaryDNSService) Sync(ctx context.Context) (AuxiliaryDNSSyncResult, error) {
	if s.nodeViewRepo == nil {
		return AuxiliaryDNSSyncResult{}, fmt.Errorf("node view repository is not configured")
	}
	if s.recordManager == nil {
		return AuxiliaryDNSSyncResult{}, fmt.Errorf("dns record manager is not configured")
	}
	if s.zone == "" {
		return AuxiliaryDNSSyncResult{}, fmt.Errorf("auxiliary dns zone is required")
	}

	nodes, err := s.nodeViewRepo.List(ctx)
	if err != nil {
		return AuxiliaryDNSSyncResult{}, fmt.Errorf("list nodes for auxiliary dns sync: %w", err)
	}
	expected := s.expectedRecords(nodes)

	existingRecords, err := s.recordManager.ListRecords(ctx, s.dnsConfig.AuxiliaryRecordType)
	if err != nil {
		return AuxiliaryDNSSyncResult{}, err
	}

	result := AuxiliaryDNSSyncResult{
		Expected: len(expected),
		Existing: len(existingRecords),
	}
	existingByKey := make(map[string][]platformdns.ManagedRecord, len(existingRecords))
	for _, record := range existingRecords {
		if !s.recordBelongsToAuxiliaryZone(record.Name) || !strings.EqualFold(strings.TrimSpace(record.Type), s.dnsConfig.AuxiliaryRecordType) {
			result.Skipped++
			continue
		}
		key := auxiliaryDNSKey(record.Name, record.Content)
		if _, ok := expected[key]; !ok {
			if err := s.recordManager.DeleteRecord(ctx, record.ID); err != nil {
				return result, fmt.Errorf("delete auxiliary dns record %s: %w", record.Name, err)
			}
			result.Deleted++
			continue
		}
		existingByKey[key] = append(existingByKey[key], record)
	}

	for key, expectation := range expected {
		matches := existingByKey[key]
		if len(matches) == 0 {
			if err := s.recordManager.CreateRecord(ctx, expectation.record); err != nil {
				return result, fmt.Errorf("create auxiliary dns record %s: %w", expectation.name, err)
			}
			result.Created++
			continue
		}
		for _, match := range matches {
			if match.TTL == expectation.record.TTL && match.Proxied == expectation.record.Proxied {
				continue
			}
			if err := s.recordManager.UpdateRecord(ctx, match.ID, expectation.record); err != nil {
				return result, fmt.Errorf("update auxiliary dns record %s: %w", expectation.name, err)
			}
			result.UpdatedOrEnsured++
		}
	}

	return result, nil
}

func (s *AuxiliaryDNSService) expectedRecords(nodes []node.Node) map[string]auxiliaryDNSExpectation {
	expected := make(map[string]auxiliaryDNSExpectation)
	for _, item := range nodes {
		if !strings.EqualFold(strings.TrimSpace(item.Status), node.StatusUp) {
			continue
		}
		publicIP := strings.TrimSpace(item.PublicIP)
		if publicIP == "" {
			continue
		}
		label := auxiliaryDNSShortLabel(item.Hostname)
		name := label + "." + s.zone
		record := platformdns.Record{
			Name:    name,
			Type:    s.dnsConfig.AuxiliaryRecordType,
			Content: publicIP,
			TTL:     s.dnsConfig.AuxiliaryTTL,
			Proxied: s.dnsConfig.AuxiliaryProxied,
		}
		expected[auxiliaryDNSKey(name, publicIP)] = auxiliaryDNSExpectation{
			name:    name,
			content: publicIP,
			record:  record,
		}
	}
	return expected
}

func auxiliaryDNSShortLabel(hostname string) string {
	rawName := strings.TrimSpace(hostname)
	shortName := rawName
	if idx := strings.Index(shortName, "-"); idx >= 0 {
		shortName = shortName[:idx]
	}
	if !hasAlphaNumeric(shortName) {
		shortName = rawName
	}
	if !hasAlphaNumeric(shortName) {
		shortName = "node"
	}
	return normalizeAuxiliaryDNSLabel(shortName)
}

func normalizeAuxiliaryDNSLabel(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var builder strings.Builder
	lastDash := false
	for _, ch := range raw {
		isAllowed := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		if isAllowed {
			builder.WriteRune(ch)
			lastDash = false
			continue
		}
		if ch == '-' || !isAllowed {
			if builder.Len() == 0 || lastDash {
				continue
			}
			builder.WriteByte('-')
			lastDash = true
		}
	}
	normalized := strings.Trim(builder.String(), "-")
	if len(normalized) > 52 {
		normalized = strings.TrimRight(normalized[:52], "-")
	}
	if normalized == "" {
		return "node"
	}
	return normalized
}

func hasAlphaNumeric(value string) bool {
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			return true
		}
	}
	return false
}

func (s *AuxiliaryDNSService) recordBelongsToAuxiliaryZone(name string) bool {
	normalizedName := strings.Trim(strings.ToLower(strings.TrimSpace(name)), ".")
	normalizedZone := strings.ToLower(s.zone)
	return normalizedName == normalizedZone || strings.HasSuffix(normalizedName, "."+normalizedZone)
}

func auxiliaryDNSKey(name, content string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(name)), ".") + "\x00" + strings.TrimSpace(content)
}
