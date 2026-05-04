package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
)

type stubAuxiliaryNodeViewRepo struct {
	items []node.Node
}

func (r stubAuxiliaryNodeViewRepo) List(context.Context) ([]node.Node, error) {
	return append([]node.Node(nil), r.items...), nil
}

func (r stubAuxiliaryNodeViewRepo) FindByUUID(context.Context, string) (*node.Node, error) {
	return nil, nil
}

func (r stubAuxiliaryNodeViewRepo) ListRegions(context.Context) ([]node.RegionSummary, error) {
	return nil, nil
}

func (r stubAuxiliaryNodeViewRepo) ListRegionZones(context.Context) ([]node.RegionZoneSummary, error) {
	return nil, nil
}

type stubAuxiliaryRecordManager struct {
	records []platformdns.ManagedRecord
	created []platformdns.Record
	updated []stubAuxiliaryRecordUpdate
	deleted []string
}

type stubAuxiliaryRecordUpdate struct {
	id     string
	record platformdns.Record
}

func (m *stubAuxiliaryRecordManager) ListRecords(context.Context, string) ([]platformdns.ManagedRecord, error) {
	return append([]platformdns.ManagedRecord(nil), m.records...), nil
}

func (m *stubAuxiliaryRecordManager) CreateRecord(_ context.Context, record platformdns.Record) error {
	m.created = append(m.created, record)
	return nil
}

func (m *stubAuxiliaryRecordManager) UpdateRecord(_ context.Context, id string, record platformdns.Record) error {
	m.updated = append(m.updated, stubAuxiliaryRecordUpdate{id: id, record: record})
	return nil
}

func (m *stubAuxiliaryRecordManager) DeleteRecord(_ context.Context, id string) error {
	m.deleted = append(m.deleted, id)
	return nil
}

func TestAuxiliaryDNSShortLabelMatchesSetupScriptRule(t *testing.T) {
	tests := map[string]string{
		"TOKY-CONTABO-6V12G-SERVER-01": "toky",
		"SG-EDGE-01":                   "sg",
		"***":                          "node",
		"Tokyo Edge":                   "tokyo-edge",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	for input, expected := range tests {
		if got := auxiliaryDNSShortLabel(input); got != expected {
			t.Fatalf("expected %q -> %q, got %q", input, expected, got)
		}
	}
}

func TestAuxiliaryDNSSyncCreatesUpdatesDeletesAndPreservesDuplicateNames(t *testing.T) {
	repo := stubAuxiliaryNodeViewRepo{items: []node.Node{
		{Hostname: "TOKY-CONTABO-6V12G-SERVER-01", PublicIP: "1.1.1.1", Status: node.StatusUp},
		{Hostname: "TOKY-CONTABO-6V12G-SERVER-02", PublicIP: "2.2.2.2", Status: node.StatusUp},
		{Hostname: "KYOT-CONTABO-6V12G-SERVER-01", PublicIP: "3.3.3.3", Status: node.StatusUp},
		{Hostname: "OSAK-CONTABO-6V12G-SERVER-01", PublicIP: "4.4.4.4", Status: node.StatusDown},
		{Hostname: "NOIP-CONTABO-6V12G-SERVER-01", Status: node.StatusUp},
	}}
	manager := &stubAuxiliaryRecordManager{records: []platformdns.ManagedRecord{
		{ID: "toky-1", Name: "toky.example.org", Type: "A", Content: "1.1.1.1", TTL: 60, Proxied: false},
		{ID: "toky-2", Name: "toky.example.org", Type: "A", Content: "2.2.2.2", TTL: 1, Proxied: false},
		{ID: "osak-1", Name: "osak.example.org", Type: "A", Content: "4.4.4.4", TTL: 1, Proxied: false},
		{ID: "stale-1", Name: "stale.example.org", Type: "A", Content: "5.5.5.5", TTL: 1, Proxied: false},
	}}
	service := NewAuxiliaryDNSService(repo, manager, config.DNSConfig{
		AuxiliaryZone:       "example.org",
		AuxiliaryRecordType: "A",
		AuxiliaryTTL:        1,
		AuxiliaryProxied:    false,
	})

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result.Expected != 3 || result.Existing != 4 || result.Created != 1 || result.UpdatedOrEnsured != 1 || result.Deleted != 2 {
		raw, _ := json.Marshal(result)
		t.Fatalf("unexpected result: %s", raw)
	}
	if len(manager.created) != 1 || manager.created[0].Name != "kyot.example.org" || manager.created[0].Content != "3.3.3.3" {
		t.Fatalf("expected kyot.example.org creation, got %#v", manager.created)
	}
	if len(manager.updated) != 1 || manager.updated[0].id != "toky-1" || manager.updated[0].record.Name != "toky.example.org" {
		t.Fatalf("expected toky-1 TTL correction, got %#v", manager.updated)
	}
	if len(manager.deleted) != 2 || manager.deleted[0] != "osak-1" || manager.deleted[1] != "stale-1" {
		t.Fatalf("expected osak and stale deletes, got %#v", manager.deleted)
	}
}
