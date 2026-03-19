package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/dnsbinding"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/region"
	"github.com/SamuelFan1/Axis/internal/domain/zone"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
	"github.com/SamuelFan1/Axis/internal/platform/workeradmin"
	"github.com/SamuelFan1/Axis/internal/repository"
)

const testNodeUUID = "11111111-1111-1111-1111-111111111111"

type stubNodeRepository struct {
	items []node.Node
	err   error

	nodes               map[string]node.Node
	saveDNSBindingCalls []dnsBindingCall
	archiveCalls        []archiveCall
	updateStatusCalls   []dnsBindingCall
}

type dnsBindingCall struct {
	UUID  string
	Label string
	Name  string
}

type archiveCall struct {
	ManagementAddress string
	ReplacedByUUID    string
	Reason            string
}

func (r *stubNodeRepository) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubNodeRepository) FindActiveByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error) {
	for _, item := range r.nodes {
		if item.ManagementAddress == managementAddress {
			copied := item
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *stubNodeRepository) FindByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error) {
	return r.FindActiveByManagementAddress(ctx, managementAddress)
}

func (r *stubNodeRepository) FindByUUID(ctx context.Context, uuid string) (*node.Node, error) {
	if item, ok := r.nodes[uuid]; ok {
		copied := item
		return &copied, nil
	}
	return nil, nil
}

func (r *stubNodeRepository) Upsert(ctx context.Context, item node.Node) error {
	if r.nodes == nil {
		r.nodes = make(map[string]node.Node)
	}
	r.nodes[item.UUID] = item
	return nil
}

func (r *stubNodeRepository) UpdateHeartbeat(ctx context.Context, item node.Node) error {
	if r.nodes == nil {
		return nil
	}

	existing, ok := r.nodes[item.UUID]
	if !ok {
		return sql.ErrNoRows
	}

	item.DNSLabel = existing.DNSLabel
	item.DNSName = existing.DNSName
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	item.LastSeenAt = item.UpdatedAt
	item.LastReportedAt = item.UpdatedAt
	r.nodes[item.UUID] = item
	return nil
}

func (r *stubNodeRepository) SaveDNSBinding(ctx context.Context, uuid string, label string, name string) error {
	existing, ok := r.nodes[uuid]
	if !ok {
		return sql.ErrNoRows
	}
	existing.DNSLabel = label
	existing.DNSName = name
	existing.UpdatedAt = time.Now().UTC()
	r.nodes[uuid] = existing
	r.saveDNSBindingCalls = append(r.saveDNSBindingCalls, dnsBindingCall{
		UUID:  uuid,
		Label: label,
		Name:  name,
	})
	return nil
}

func (r *stubNodeRepository) ArchiveAndDeleteByManagementAddress(ctx context.Context, managementAddress string, replacedByUUID string, reason string) error {
	for uuidValue, item := range r.nodes {
		if item.ManagementAddress != managementAddress {
			continue
		}
		delete(r.nodes, uuidValue)
		r.archiveCalls = append(r.archiveCalls, archiveCall{
			ManagementAddress: managementAddress,
			ReplacedByUUID:    replacedByUUID,
			Reason:            reason,
		})
		return nil
	}
	return nil
}

func (r *stubNodeRepository) List(ctx context.Context) ([]node.Node, error) {
	if r.err != nil {
		return nil, r.err
	}
	if len(r.items) == 0 && len(r.nodes) > 0 {
		items := make([]node.Node, 0, len(r.nodes))
		for _, item := range r.nodes {
			items = append(items, item)
		}
		return items, nil
	}
	return r.items, nil
}

func (r *stubNodeRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	return false, nil
}

func (r *stubNodeRepository) DeleteByRegionUUID(ctx context.Context, regionUUID string) (int64, error) {
	return 0, nil
}

func (r *stubNodeRepository) DeleteByZoneUUID(ctx context.Context, zoneUUID string) (int64, error) {
	return 0, nil
}

func (r *stubNodeRepository) UpdateStatus(ctx context.Context, uuid string, status string) (bool, error) {
	r.updateStatusCalls = append(r.updateStatusCalls, dnsBindingCall{UUID: uuid, Label: status})
	return false, nil
}

func (r *stubNodeRepository) ListRegions(ctx context.Context) ([]node.RegionSummary, error) {
	return nil, nil
}

func (r *stubNodeRepository) ListRegionZones(ctx context.Context) ([]node.RegionZoneSummary, error) {
	return nil, nil
}

func (r *stubNodeRepository) MarkTimedOutNodesDown(ctx context.Context, localRegion string, timeoutSec int) (int, error) {
	return 0, nil
}

type stubRegionRepository struct{}

func (r *stubRegionRepository) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubRegionRepository) Create(ctx context.Context, name string) (region.Region, error) {
	return region.Region{}, nil
}

func (r *stubRegionRepository) List(ctx context.Context) ([]region.RegionListItem, error) {
	return nil, nil
}

func (r *stubRegionRepository) FindByUUID(ctx context.Context, uuid string) (*region.Region, error) {
	return nil, nil
}

func (r *stubRegionRepository) FindByName(ctx context.Context, name string) (*region.Region, error) {
	if name == "" {
		return nil, nil
	}
	return &region.Region{
		UUID: "region-" + name,
		Name: name,
	}, nil
}

func (r *stubRegionRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	return false, nil
}

func (r *stubRegionRepository) DeleteNodesByRegionUUID(ctx context.Context, regionUUID string) (int64, error) {
	return 0, nil
}

func (r *stubRegionRepository) MigrateNodesRegionUUID(ctx context.Context) error {
	return nil
}

type stubZoneRepository struct{}

func (r *stubZoneRepository) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubZoneRepository) EnsureConstraints(ctx context.Context) error {
	return nil
}

func (r *stubZoneRepository) Create(ctx context.Context, regionUUID string, name string) (zone.Zone, error) {
	return zone.Zone{}, nil
}

func (r *stubZoneRepository) List(ctx context.Context) ([]zone.ZoneListItem, error) {
	return nil, nil
}

func (r *stubZoneRepository) FindByUUID(ctx context.Context, uuid string) (*zone.Zone, error) {
	return nil, nil
}

func (r *stubZoneRepository) FindByRegionUUIDAndName(ctx context.Context, regionUUID string, name string) (*zone.Zone, error) {
	if regionUUID == "" || name == "" {
		return nil, nil
	}
	return &zone.Zone{
		UUID:       "zone-" + name,
		RegionUUID: regionUUID,
		Name:       name,
	}, nil
}

func (r *stubZoneRepository) CountByRegionUUID(ctx context.Context, regionUUID string) (int, error) {
	return 0, nil
}

func (r *stubZoneRepository) DeleteByUUID(ctx context.Context, uuid string) (bool, error) {
	return false, nil
}

func (r *stubZoneRepository) DeleteNodesByZoneUUID(ctx context.Context, zoneUUID string) (int64, error) {
	return 0, nil
}

func (r *stubZoneRepository) MigrateNodesZoneUUID(ctx context.Context) error {
	return nil
}

type stubWorkerAdminClient struct {
	enabled      bool
	disableCalls []string
	enableCalls  []string
	disableErr   error
	enableErr    error
}

func (c *stubWorkerAdminClient) Enabled() bool {
	return c != nil && c.enabled
}

func (c *stubWorkerAdminClient) DisableNode(ctx context.Context, originLabel string) error {
	c.disableCalls = append(c.disableCalls, originLabel)
	if c.disableErr != nil {
		return c.disableErr
	}
	return nil
}

func (c *stubWorkerAdminClient) EnableNode(ctx context.Context, originLabel string) error {
	c.enableCalls = append(c.enableCalls, originLabel)
	if c.enableErr != nil {
		return c.enableErr
	}
	return nil
}

func newTestNodeService(items []node.Node) *NodeService {
	return &NodeService{
		repo:       &stubNodeRepository{items: items},
		regionRepo: &stubRegionRepository{},
		zoneRepo:   &stubZoneRepository{},
		regionConfig: config.RegionConfig{
			Regions: []string{"asia", "europe"},
			RegionZones: map[string][]string{
				"asia":   {"SG", "JP"},
				"europe": {"DE"},
			},
		},
		workerAdmin: &stubWorkerAdminClient{enabled: true},
	}
}

type stubDNSBindingRepository struct {
	bindings          map[string]dnsbinding.Binding
	getErr            error
	allocateErr       error
	updateErr         error
	allocateNext      int
	allocateCalls     []string
	updatePublicIPLog []string
}

func (r *stubDNSBindingRepository) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubDNSBindingRepository) GetByNodeUUID(ctx context.Context, nodeUUID string) (*dnsbinding.Binding, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if binding, ok := r.bindings[nodeUUID]; ok {
		copied := binding
		return &copied, nil
	}
	return nil, nil
}

func (r *stubDNSBindingRepository) GetByDNSLabel(ctx context.Context, label string) (*dnsbinding.Binding, error) {
	for _, binding := range r.bindings {
		if binding.DNSLabel == label {
			copied := binding
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *stubDNSBindingRepository) AllocateForNode(ctx context.Context, nodeUUID string, zone string, prefix string) (*dnsbinding.Binding, error) {
	if r.allocateErr != nil {
		return nil, r.allocateErr
	}
	if binding, ok := r.bindings[nodeUUID]; ok {
		copied := binding
		return &copied, nil
	}
	if r.bindings == nil {
		r.bindings = make(map[string]dnsbinding.Binding)
	}
	if r.allocateNext <= 0 {
		r.allocateNext = 1
	}
	sequence := r.allocateNext
	r.allocateNext++
	label := platformdns.BuildDNSLabel(prefix, sequence)
	binding := dnsbinding.Binding{
		NodeUUID:     nodeUUID,
		DNSLabel:     label,
		DNSName:      platformdns.BuildDNSName(label, zone),
		Zone:         zone,
		RecordPrefix: prefix,
		Sequence:     sequence,
	}
	r.bindings[nodeUUID] = binding
	r.allocateCalls = append(r.allocateCalls, nodeUUID)
	copied := binding
	return &copied, nil
}

func (r *stubDNSBindingRepository) UpdateLastPublicIP(ctx context.Context, nodeUUID string, publicIP string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	binding, ok := r.bindings[nodeUUID]
	if !ok {
		return sql.ErrNoRows
	}
	binding.LastPublicIP = publicIP
	r.bindings[nodeUUID] = binding
	r.updatePublicIPLog = append(r.updatePublicIPLog, publicIP)
	return nil
}

func (r *stubDNSBindingRepository) SeedFromManagedNodes(ctx context.Context, zone string, prefix string) (repository.DNSBindingSeedResult, error) {
	return repository.DNSBindingSeedResult{}, nil
}

func (r *stubDNSBindingRepository) EnsureCounterFloor(ctx context.Context, zone string, prefix string, floor int) error {
	if r.allocateNext < floor+1 {
		r.allocateNext = floor + 1
	}
	return nil
}

type stubDNSProvider struct {
	err     error
	calls   []platformdns.Record
	enabled bool
}

func (p *stubDNSProvider) EnsureRecord(ctx context.Context, record platformdns.Record) error {
	p.calls = append(p.calls, record)
	if p.err != nil {
		return p.err
	}
	return nil
}

func (p *stubDNSProvider) Enabled() bool {
	return p.enabled
}

func newDNSNodeService(repo *stubNodeRepository, bindingRepo *stubDNSBindingRepository, provider *stubDNSProvider) *NodeService {
	if provider == nil {
		provider = &stubDNSProvider{enabled: true}
	}
	return NewNodeService(
		repo,
		&stubRegionRepository{},
		&stubZoneRepository{},
		provider,
		bindingRepo,
		config.DNSConfig{
			Enabled:      true,
			Provider:     "cloudflare",
			Zone:         "example.com",
			RecordPrefix: "dl-",
			RecordType:   "A",
			TTL:          1,
			Proxied:      false,
		},
		config.RegionConfig{
			Regions: []string{"asia"},
			RegionZones: map[string][]string{
				"asia": {"SG"},
			},
		},
		&stubWorkerAdminClient{enabled: true},
	)
}

func newDNSRepository(items ...node.Node) *stubNodeRepository {
	repo := &stubNodeRepository{
		nodes: make(map[string]node.Node, len(items)),
	}
	for _, item := range items {
		repo.nodes[item.UUID] = item
	}
	return repo
}

func newReportInput(publicIP string) node.Node {
	return node.Node{
		UUID:              testNodeUUID,
		Hostname:          "node-1",
		ManagementAddress: "10.0.0.1:9090",
		InternalIP:        "10.0.0.1",
		PublicIP:          publicIP,
		Region:            "asia",
		Zone:              "SG",
		Status:            node.StatusUp,
	}
}

func newPolicyNodeService(repo *stubNodeRepository, requireTunnel bool) *NodeService {
	if repo == nil {
		repo = newDNSRepository(newReportInput(""))
	}
	return NewNodeService(
		repo,
		&stubRegionRepository{},
		&stubZoneRepository{},
		nil,
		nil,
		config.DNSConfig{
			RequireCFTunnelHealth: requireTunnel,
			CFTunnelSourceName:    "cloudflared",
		},
		config.RegionConfig{
			Regions: []string{"asia"},
			RegionZones: map[string][]string{
				"asia": {"SG"},
			},
		},
		&stubWorkerAdminClient{enabled: true},
	)
}

func TestSetStatusDisablesExternalMaintenanceWithoutUpdatingNodeStatus(t *testing.T) {
	repo := newDNSRepository(node.Node{
		UUID:     testNodeUUID,
		Hostname: "SGP-DIGITALOCEAN-2V8G-SERVER-01",
		Region:   "asia",
		Zone:     "SG",
		Status:   node.StatusUp,
	})
	workerClient := &stubWorkerAdminClient{enabled: true}
	svc := NewNodeService(
		repo,
		&stubRegionRepository{},
		&stubZoneRepository{},
		nil,
		nil,
		config.DNSConfig{},
		config.RegionConfig{},
		workerClient,
	)

	result, err := svc.SetStatus(context.Background(), testNodeUUID, node.StatusDown)
	if err != nil {
		t.Fatalf("SetStatus returned error: %v", err)
	}
	if !result.ExternalMaintenance || !result.WorkerSynced {
		t.Fatalf("expected external maintenance synced, got %+v", result)
	}
	if result.Node.Status != node.StatusUp {
		t.Fatalf("expected internal status unchanged, got %s", result.Node.Status)
	}
	if len(workerClient.disableCalls) != 1 || workerClient.disableCalls[0] != "api-origin-sgp-digitalocean-2v8g-server-01" {
		t.Fatalf("unexpected disable calls: %+v", workerClient.disableCalls)
	}
	if len(repo.updateStatusCalls) != 0 {
		t.Fatalf("expected no repo UpdateStatus calls, got %+v", repo.updateStatusCalls)
	}
}

func TestSetStatusEnablesExternalMaintenanceWithoutUpdatingNodeStatus(t *testing.T) {
	repo := newDNSRepository(node.Node{
		UUID:     testNodeUUID,
		Hostname: "DEFF-DIGITALOCEAN-2V8G-SERVER-03",
		Region:   "europe",
		Zone:     "DE",
		Status:   node.StatusDown,
	})
	workerClient := &stubWorkerAdminClient{enabled: true}
	svc := NewNodeService(
		repo,
		&stubRegionRepository{},
		&stubZoneRepository{},
		nil,
		nil,
		config.DNSConfig{},
		config.RegionConfig{},
		workerClient,
	)

	result, err := svc.SetStatus(context.Background(), testNodeUUID, node.StatusUp)
	if err != nil {
		t.Fatalf("SetStatus returned error: %v", err)
	}
	if result.ExternalMaintenance || !result.WorkerSynced {
		t.Fatalf("expected external maintenance cleared, got %+v", result)
	}
	if len(workerClient.enableCalls) != 1 || workerClient.enableCalls[0] != "api-origin-deff-digitalocean-2v8g-server-03" {
		t.Fatalf("unexpected enable calls: %+v", workerClient.enableCalls)
	}
	if len(repo.updateStatusCalls) != 0 {
		t.Fatalf("expected no repo UpdateStatus calls, got %+v", repo.updateStatusCalls)
	}
}

func TestSetStatusReturnsWorkerAdminError(t *testing.T) {
	repo := newDNSRepository(node.Node{
		UUID:     testNodeUUID,
		Hostname: "SGP-DIGITALOCEAN-2V8G-SERVER-01",
		Status:   node.StatusUp,
	})
	workerClient := &stubWorkerAdminClient{enabled: true, disableErr: fmt.Errorf("worker request failed")}
	svc := NewNodeService(
		repo,
		&stubRegionRepository{},
		&stubZoneRepository{},
		nil,
		nil,
		config.DNSConfig{},
		config.RegionConfig{},
		workerClient,
	)

	_, err := svc.SetStatus(context.Background(), testNodeUUID, node.StatusDown)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "worker request failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

var _ workeradmin.Client = (*stubWorkerAdminClient)(nil)

func monitoringSnapshot(t *testing.T, sources ...map[string]string) json.RawMessage {
	t.Helper()
	payload := map[string]interface{}{
		"schema_version": "1",
		"sources":        sources,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal monitoring snapshot: %v", err)
	}
	return raw
}

func TestAssignByRegionZonePrefersZoneLowestScore(t *testing.T) {
	svc := newTestNodeService([]node.Node{
		{UUID: "zone-high", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 90, CPUUsagePercent: 50, MemoryUsagePercent: 40},
		{UUID: "zone-low", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 30, CPUUsagePercent: 20, MemoryUsagePercent: 10},
		{UUID: "region-lower", Region: "asia", Zone: "JP", Status: node.StatusUp, DiskUsagePercent: 1, CPUUsagePercent: 1, MemoryUsagePercent: 1},
	})

	item, err := svc.AssignByRegionZone(context.Background(), "asia", "SG")
	if err != nil {
		t.Fatalf("AssignByRegionZone returned error: %v", err)
	}
	if item.UUID != "zone-low" {
		t.Fatalf("expected zone-low, got %s", item.UUID)
	}
}

func TestAssignByRegionZoneFallsBackToRegion(t *testing.T) {
	svc := newTestNodeService([]node.Node{
		{UUID: "zone-down", Region: "asia", Zone: "SG", Status: node.StatusDown, DiskUsagePercent: 10, CPUUsagePercent: 10, MemoryUsagePercent: 10},
		{UUID: "region-up", Region: "asia", Zone: "JP", Status: node.StatusUp, DiskUsagePercent: 20, CPUUsagePercent: 20, MemoryUsagePercent: 20},
	})

	item, err := svc.AssignByRegionZone(context.Background(), "asia", "SG")
	if err != nil {
		t.Fatalf("AssignByRegionZone returned error: %v", err)
	}
	if item.UUID != "region-up" {
		t.Fatalf("expected region-up, got %s", item.UUID)
	}
}

func TestAssignByRegionZoneReturnsNotFoundWithoutUpNodes(t *testing.T) {
	svc := newTestNodeService([]node.Node{
		{UUID: "zone-down", Region: "asia", Zone: "SG", Status: node.StatusDown},
		{UUID: "region-down", Region: "asia", Zone: "JP", Status: node.StatusDown},
		{UUID: "other-region-up", Region: "europe", Zone: "DE", Status: node.StatusUp},
	})

	_, err := svc.AssignByRegionZone(context.Background(), "asia", "SG")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "node not found" {
		t.Fatalf("expected node not found, got %v", err)
	}
}

func TestAssignByRegionZoneReturnsOneOfLowestScoreTies(t *testing.T) {
	svc := newTestNodeService([]node.Node{
		{UUID: "best-a", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 20, CPUUsagePercent: 20, MemoryUsagePercent: 20},
		{UUID: "best-b", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 20, CPUUsagePercent: 20, MemoryUsagePercent: 20},
		{UUID: "worse", Region: "asia", Zone: "SG", Status: node.StatusUp, DiskUsagePercent: 60, CPUUsagePercent: 60, MemoryUsagePercent: 60},
	})

	item, err := svc.AssignByRegionZone(context.Background(), "asia", "SG")
	if err != nil {
		t.Fatalf("AssignByRegionZone returned error: %v", err)
	}
	if item.UUID != "best-a" && item.UUID != "best-b" {
		t.Fatalf("expected one of best-a or best-b, got %s", item.UUID)
	}
}

func TestAssignByRegionZoneValidatesRegionZone(t *testing.T) {
	svc := newTestNodeService(nil)

	_, err := svc.AssignByRegionZone(context.Background(), "asia", "CN")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != `zone "CN" is not allowed for region "asia"` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportAssignsNewDNSBindingWithoutAuthorityBinding(t *testing.T) {
	repo := newDNSRepository(newReportInput(""))
	bindingRepo := &stubDNSBindingRepository{allocateNext: 1}
	provider := &stubDNSProvider{enabled: true}
	svc := newDNSNodeService(repo, bindingRepo, provider)

	item, err := svc.Report(context.Background(), newReportInput("1.1.1.1"))
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.DNSLabel != "dl-001" || item.DNSName != "dl-001.example.com" {
		t.Fatalf("expected dl-001.example.com, got %s / %s", item.DNSLabel, item.DNSName)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(provider.calls))
	}
	if provider.calls[0].Name != "dl-001.example.com" {
		t.Fatalf("unexpected provider name: %s", provider.calls[0].Name)
	}
	if len(repo.saveDNSBindingCalls) != 1 {
		t.Fatalf("expected 1 dns binding save call, got %d", len(repo.saveDNSBindingCalls))
	}
	if binding, ok := bindingRepo.bindings[testNodeUUID]; !ok {
		t.Fatal("expected authority binding to be saved")
	} else if binding.LastPublicIP != "1.1.1.1" {
		t.Fatalf("expected authority binding public ip 1.1.1.1, got %s", binding.LastPublicIP)
	}
}

func TestReportReusesExistingDNSBindingWhenPublicIPChanges(t *testing.T) {
	initial := newReportInput("")
	initial.DNSLabel = "dl-007"
	initial.DNSName = "dl-007.example.com"
	repo := newDNSRepository(initial)
	bindingRepo := &stubDNSBindingRepository{
		bindings: map[string]dnsbinding.Binding{
			testNodeUUID: {
				NodeUUID:     testNodeUUID,
				DNSLabel:     "dl-007",
				DNSName:      "dl-007.example.com",
				Zone:         "example.com",
				RecordPrefix: "dl-",
				Sequence:     7,
				LastPublicIP: "1.1.1.1",
			},
		},
	}
	provider := &stubDNSProvider{enabled: true}
	svc := newDNSNodeService(repo, bindingRepo, provider)

	item, err := svc.Report(context.Background(), newReportInput("2.2.2.2"))
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.DNSLabel != "dl-007" || item.DNSName != "dl-007.example.com" {
		t.Fatalf("expected existing binding, got %s / %s", item.DNSLabel, item.DNSName)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(provider.calls))
	}
	if provider.calls[0].Name != "dl-007.example.com" || provider.calls[0].Content != "2.2.2.2" {
		t.Fatalf("expected provider update for dl-007.example.com -> 2.2.2.2, got %+v", provider.calls[0])
	}
	if len(repo.saveDNSBindingCalls) != 1 {
		t.Fatalf("expected 1 dns binding save call, got %d", len(repo.saveDNSBindingCalls))
	}
	if binding := bindingRepo.bindings[testNodeUUID]; binding.LastPublicIP != "2.2.2.2" {
		t.Fatalf("expected authority binding public ip 2.2.2.2, got %s", binding.LastPublicIP)
	}
}

func TestReportAllocatesWhenAuthorityBindingMissingEvenIfDBHasOldBinding(t *testing.T) {
	initial := newReportInput("")
	initial.DNSLabel = "dl-010"
	initial.DNSName = "dl-010.example.com"
	repo := newDNSRepository(initial)
	bindingRepo := &stubDNSBindingRepository{allocateNext: 11}
	provider := &stubDNSProvider{enabled: true}
	svc := newDNSNodeService(repo, bindingRepo, provider)

	item, err := svc.Report(context.Background(), newReportInput("5.5.5.5"))
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.DNSLabel != "dl-011" || item.DNSName != "dl-011.example.com" {
		t.Fatalf("expected allocated binding dl-011.example.com, got %s / %s", item.DNSLabel, item.DNSName)
	}
	if len(provider.calls) != 1 || provider.calls[0].Name != "dl-011.example.com" {
		t.Fatalf("unexpected provider calls: %+v", provider.calls)
	}
}

func TestReportReturnsErrorWhenEnsureRecordFails(t *testing.T) {
	repo := newDNSRepository(newReportInput(""))
	bindingRepo := &stubDNSBindingRepository{allocateNext: 8}
	provider := &stubDNSProvider{enabled: true, err: errors.New("cloudflare down")}
	svc := newDNSNodeService(repo, bindingRepo, provider)

	if _, err := svc.Report(context.Background(), newReportInput("2.2.2.2")); err == nil {
		t.Fatal("expected ensure record error, got nil")
	}
	if len(provider.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(provider.calls))
	}
	if len(repo.saveDNSBindingCalls) != 0 {
		t.Fatalf("expected no dns binding mirror save, got %d", len(repo.saveDNSBindingCalls))
	}
}

func TestReportKeepsStatusWhenTunnelPolicyDisabled(t *testing.T) {
	repo := newDNSRepository(newReportInput(""))
	svc := newPolicyNodeService(repo, false)
	input := newReportInput("")
	input.MonitoringSnapshot = monitoringSnapshot(t, map[string]string{
		"name":   "cloudflared",
		"status": "error",
	})

	item, err := svc.Report(context.Background(), input)
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.Status != node.StatusUp {
		t.Fatalf("expected status up when policy disabled, got %s", item.Status)
	}
}

func TestReportForcesDownWhenTunnelSourceMissing(t *testing.T) {
	repo := newDNSRepository(newReportInput(""))
	svc := newPolicyNodeService(repo, true)
	input := newReportInput("")
	input.MonitoringSnapshot = monitoringSnapshot(t, map[string]string{
		"name":   "go-sidecar",
		"status": "ok",
	})

	item, err := svc.Report(context.Background(), input)
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.Status != node.StatusDown {
		t.Fatalf("expected status down when tunnel source missing, got %s", item.Status)
	}
}

func TestReportForcesDownWhenTunnelSourceErrors(t *testing.T) {
	repo := newDNSRepository(newReportInput(""))
	svc := newPolicyNodeService(repo, true)
	input := newReportInput("")
	input.MonitoringSnapshot = monitoringSnapshot(t, map[string]string{
		"name":   "cloudflared",
		"status": "error",
	})

	item, err := svc.Report(context.Background(), input)
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.Status != node.StatusDown {
		t.Fatalf("expected status down when tunnel source errors, got %s", item.Status)
	}
}

func TestReportAllowsUpWhenTunnelSourceHealthy(t *testing.T) {
	repo := newDNSRepository(newReportInput(""))
	svc := newPolicyNodeService(repo, true)
	input := newReportInput("")
	input.MonitoringSnapshot = monitoringSnapshot(t, map[string]string{
		"name":   "cloudflared",
		"status": "ok",
	})

	item, err := svc.Report(context.Background(), input)
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.Status != node.StatusUp {
		t.Fatalf("expected status up when tunnel source is healthy, got %s", item.Status)
	}
}

func TestRegisterReplacesOldActiveNodeWhenManagementAddressMatchesNewUUID(t *testing.T) {
	oldNode := node.Node{
		UUID:              "22222222-2222-2222-2222-222222222222",
		Hostname:          "node-old",
		ManagementAddress: "10.0.0.1:9090",
		Region:            "asia",
		Zone:              "SG",
		Status:            node.StatusUp,
		DNSLabel:          "dl-001",
		DNSName:           "dl-001.example.com",
	}
	repo := newDNSRepository(oldNode)
	svc := newDNSNodeService(repo, &stubDNSBindingRepository{}, nil)

	registered, err := svc.Register(context.Background(), node.Node{
		UUID:              testNodeUUID,
		Hostname:          "node-new",
		ManagementAddress: "10.0.0.1:9090",
		Region:            "asia",
		Zone:              "SG",
		Status:            node.StatusUp,
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if registered.UUID != testNodeUUID {
		t.Fatalf("expected new uuid %s, got %s", testNodeUUID, registered.UUID)
	}
	if _, ok := repo.nodes[oldNode.UUID]; ok {
		t.Fatal("expected old node to be archived and deleted from active set")
	}
	if _, ok := repo.nodes[testNodeUUID]; !ok {
		t.Fatal("expected new node to exist in active set")
	}
	if len(repo.archiveCalls) != 1 {
		t.Fatalf("expected 1 archive call, got %d", len(repo.archiveCalls))
	}
	if repo.archiveCalls[0].ReplacedByUUID != testNodeUUID {
		t.Fatalf("expected archive replacement uuid %s, got %s", testNodeUUID, repo.archiveCalls[0].ReplacedByUUID)
	}
}
