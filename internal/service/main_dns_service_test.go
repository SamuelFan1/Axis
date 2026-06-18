package service

import (
	"context"
	"testing"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
)

func TestMainDNSSyncCreatesMissingRecord(t *testing.T) {
	manager := &stubAuxiliaryRecordManager{}
	service := newTestMainDNSSyncService(
		[]node.Node{{UUID: "node-1", Status: node.StatusUp, PublicIP: "1.1.1.1"}},
		map[string]dnsbinding.Binding{
			"node-1": newTestMainDNSBinding("node-1", "dl-001"),
		},
		manager,
	)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if result.Expected != 1 || result.Created != 1 {
		t.Fatalf("expected one created record, got %+v", result)
	}
	if len(manager.created) != 1 || manager.created[0].Name != "dl-001.example.com" || manager.created[0].Content != "1.1.1.1" {
		t.Fatalf("unexpected created records: %+v", manager.created)
	}
}

func TestMainDNSSyncUpdatesRecordWhenIPChanges(t *testing.T) {
	manager := &stubAuxiliaryRecordManager{records: []platformdns.ManagedRecord{
		{ID: "record-1", Name: "dl-001.example.com", Type: "A", Content: "1.1.1.1", TTL: 1, Proxied: false},
	}}
	service := newTestMainDNSSyncService(
		[]node.Node{{UUID: "node-1", Status: node.StatusUp, PublicIP: "2.2.2.2"}},
		map[string]dnsbinding.Binding{
			"node-1": newTestMainDNSBinding("node-1", "dl-001"),
		},
		manager,
	)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if result.Expected != 1 || result.UpdatedOrEnsured != 1 {
		t.Fatalf("expected one updated record, got %+v", result)
	}
	if len(manager.updated) != 1 || manager.updated[0].id != "record-1" || manager.updated[0].record.Content != "2.2.2.2" {
		t.Fatalf("unexpected updated records: %+v", manager.updated)
	}
}

func TestMainDNSSyncSkipsDownAndNoPublicIPNodes(t *testing.T) {
	manager := &stubAuxiliaryRecordManager{}
	service := newTestMainDNSSyncService(
		[]node.Node{
			{UUID: "down-node", Status: node.StatusDown, PublicIP: "1.1.1.1"},
			{UUID: "no-ip-node", Status: node.StatusUp},
		},
		map[string]dnsbinding.Binding{
			"down-node":  newTestMainDNSBinding("down-node", "dl-001"),
			"no-ip-node": newTestMainDNSBinding("no-ip-node", "dl-002"),
		},
		manager,
	)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if result.Expected != 0 || result.Created != 0 || result.UpdatedOrEnsured != 0 {
		t.Fatalf("expected no records to sync, got %+v", result)
	}
}

func TestMainDNSSyncCountsMissingBinding(t *testing.T) {
	manager := &stubAuxiliaryRecordManager{}
	service := newTestMainDNSSyncService(
		[]node.Node{{UUID: "node-1", Status: node.StatusUp, PublicIP: "1.1.1.1"}},
		nil,
		manager,
	)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if result.Expected != 0 || result.MissingBinding != 1 {
		t.Fatalf("expected one missing binding, got %+v", result)
	}
}

func TestMainDNSSyncDoesNotDeleteExtraRecords(t *testing.T) {
	manager := &stubAuxiliaryRecordManager{records: []platformdns.ManagedRecord{
		{ID: "record-1", Name: "dl-001.example.com", Type: "A", Content: "1.1.1.1", TTL: 1, Proxied: false},
		{ID: "extra-1", Name: "dl-999.example.com", Type: "A", Content: "9.9.9.9", TTL: 1, Proxied: false},
	}}
	service := newTestMainDNSSyncService(
		[]node.Node{{UUID: "node-1", Status: node.StatusUp, PublicIP: "1.1.1.1"}},
		map[string]dnsbinding.Binding{
			"node-1": newTestMainDNSBinding("node-1", "dl-001"),
		},
		manager,
	)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if result.Expected != 1 || result.Created != 0 || result.UpdatedOrEnsured != 0 || result.Skipped != 1 {
		t.Fatalf("expected extra record to be skipped, got %+v", result)
	}
	if len(manager.deleted) != 0 {
		t.Fatalf("expected no deletes, got %+v", manager.deleted)
	}
}

func newTestMainDNSSyncService(nodes []node.Node, bindings map[string]dnsbinding.Binding, manager *stubAuxiliaryRecordManager) *MainDNSSyncService {
	return NewMainDNSSyncService(
		stubAuxiliaryNodeViewRepo{items: nodes},
		&stubDNSBindingRepository{bindings: bindings},
		manager,
		config.DNSConfig{
			Zone:         "example.com",
			RecordPrefix: "dl-",
			RecordType:   "A",
			TTL:          1,
			Proxied:      false,
		},
	)
}

func newTestMainDNSBinding(nodeUUID string, label string) dnsbinding.Binding {
	return dnsbinding.Binding{
		NodeUUID:     nodeUUID,
		DNSLabel:     label,
		DNSName:      label + ".example.com",
		Zone:         "example.com",
		RecordPrefix: "dl-",
	}
}
