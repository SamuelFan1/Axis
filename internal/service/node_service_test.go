package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
	"github.com/SamuelFan1/Axis/internal/domain/region"
	"github.com/SamuelFan1/Axis/internal/domain/zone"
	platformdns "github.com/SamuelFan1/Axis/internal/platform/dns"
)

const testNodeUUID = "11111111-1111-1111-1111-111111111111"

type stubNodeRepository struct {
	items []node.Node
	err   error

	nodes               map[string]node.Node
	saveDNSBindingCalls []dnsBindingCall
}

type dnsBindingCall struct {
	UUID  string
	Label string
	Name  string
}

func (r *stubNodeRepository) EnsureSchema(ctx context.Context) error {
	return nil
}

func (r *stubNodeRepository) FindByManagementAddress(ctx context.Context, managementAddress string) (*node.Node, error) {
	for _, item := range r.nodes {
		if item.ManagementAddress == managementAddress {
			copied := item
			return &copied, nil
		}
	}
	return nil, nil
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

func (r *stubNodeRepository) UpdateStatus(ctx context.Context, uuid string, status string) (bool, error) {
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

func (r *stubZoneRepository) Create(ctx context.Context, name string) (zone.Zone, error) {
	return zone.Zone{}, nil
}

func (r *stubZoneRepository) List(ctx context.Context) ([]zone.ZoneListItem, error) {
	return nil, nil
}

func (r *stubZoneRepository) FindByUUID(ctx context.Context, uuid string) (*zone.Zone, error) {
	return nil, nil
}

func (r *stubZoneRepository) FindByName(ctx context.Context, name string) (*zone.Zone, error) {
	if name == "" {
		return nil, nil
	}
	return &zone.Zone{
		UUID: "zone-" + name,
		Name: name,
	}, nil
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
	}
}

type stubBindingStore struct {
	bindings    map[string]platformdns.Binding
	listItems   []platformdns.Binding
	loadErr     error
	saveErr     error
	listErr     error
	reserveErr  error
	reserveNext int
	saveCalls   []platformdns.Binding
}

func (s *stubBindingStore) Load(nodeUUID string) (*platformdns.Binding, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if binding, ok := s.bindings[nodeUUID]; ok {
		copied := binding
		return &copied, nil
	}
	return nil, nil
}

func (s *stubBindingStore) Save(binding platformdns.Binding) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	if s.bindings == nil {
		s.bindings = make(map[string]platformdns.Binding)
	}
	s.bindings[binding.NodeUUID] = binding
	s.saveCalls = append(s.saveCalls, binding)
	return nil
}

func (s *stubBindingStore) List() ([]platformdns.Binding, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listItems != nil {
		items := make([]platformdns.Binding, len(s.listItems))
		copy(items, s.listItems)
		return items, nil
	}
	items := make([]platformdns.Binding, 0, len(s.bindings))
	for _, binding := range s.bindings {
		items = append(items, binding)
	}
	return items, nil
}

func (s *stubBindingStore) ReserveNextSequence(prefix string) (int, error) {
	if s.reserveErr != nil {
		return 0, s.reserveErr
	}
	if s.reserveNext <= 0 {
		s.reserveNext = 1
	}
	value := s.reserveNext
	s.reserveNext++
	return value, nil
}

type stubResolver struct {
	results map[string][]string
	errs    map[string]error
	calls   []string
}

func (r *stubResolver) LookupA(ctx context.Context, host string) ([]string, error) {
	r.calls = append(r.calls, host)
	if err, ok := r.errs[host]; ok {
		return nil, err
	}
	if result, ok := r.results[host]; ok {
		copied := make([]string, len(result))
		copy(copied, result)
		return copied, nil
	}
	return nil, nil
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

func newDNSNodeService(repo *stubNodeRepository, store *stubBindingStore, resolver *stubResolver, provider *stubDNSProvider) *NodeService {
	if provider == nil {
		provider = &stubDNSProvider{enabled: true}
	}
	return NewNodeService(
		repo,
		&stubRegionRepository{},
		&stubZoneRepository{},
		provider,
		store,
		resolver,
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

func TestReportAssignsNewDNSBindingWithoutLocalFile(t *testing.T) {
	repo := newDNSRepository(newReportInput(""))
	store := &stubBindingStore{reserveNext: 1}
	resolver := &stubResolver{}
	provider := &stubDNSProvider{enabled: true}
	svc := newDNSNodeService(repo, store, resolver, provider)

	item, err := svc.Report(context.Background(), newReportInput("1.1.1.1"))
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.DNSLabel != "dl-001" {
		t.Fatalf("expected dl-001, got %s", item.DNSLabel)
	}
	if item.DNSName != "dl-001.example.com" {
		t.Fatalf("expected dl-001.example.com, got %s", item.DNSName)
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
	if binding, ok := store.bindings[testNodeUUID]; !ok {
		t.Fatal("expected local binding to be saved")
	} else if binding.LastPublicIP != "1.1.1.1" {
		t.Fatalf("expected local binding public ip 1.1.1.1, got %s", binding.LastPublicIP)
	}
}

func TestReportKeepsExistingDNSBindingWhenLookupMatchesPublicIP(t *testing.T) {
	repo := newDNSRepository(newReportInput(""))
	store := &stubBindingStore{
		bindings: map[string]platformdns.Binding{
			testNodeUUID: {
				NodeUUID:     testNodeUUID,
				DNSLabel:     "dl-007",
				DNSName:      "dl-007.example.com",
				LastPublicIP: "9.9.9.9",
				UpdatedAt:    time.Now().Add(-time.Hour),
			},
		},
	}
	resolver := &stubResolver{
		results: map[string][]string{
			"dl-007.example.com": {"2.2.2.2", "3.3.3.3"},
		},
	}
	provider := &stubDNSProvider{enabled: true}
	svc := newDNSNodeService(repo, store, resolver, provider)

	item, err := svc.Report(context.Background(), newReportInput("2.2.2.2"))
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.DNSLabel != "dl-007" || item.DNSName != "dl-007.example.com" {
		t.Fatalf("expected existing binding, got %s / %s", item.DNSLabel, item.DNSName)
	}
	if len(provider.calls) != 0 {
		t.Fatalf("expected no provider call, got %d", len(provider.calls))
	}
	if len(repo.saveDNSBindingCalls) != 1 {
		t.Fatalf("expected 1 dns binding save call, got %d", len(repo.saveDNSBindingCalls))
	}
	if binding := store.bindings[testNodeUUID]; binding.LastPublicIP != "2.2.2.2" {
		t.Fatalf("expected local binding public ip 2.2.2.2, got %s", binding.LastPublicIP)
	}
}

func TestReportRotatesDNSBindingWhenLookupMismatchesPublicIP(t *testing.T) {
	initial := newReportInput("")
	initial.DNSLabel = "dl-007"
	initial.DNSName = "dl-007.example.com"
	repo := newDNSRepository(initial)
	store := &stubBindingStore{
		bindings: map[string]platformdns.Binding{
			testNodeUUID: {
				NodeUUID:     testNodeUUID,
				DNSLabel:     "dl-007",
				DNSName:      "dl-007.example.com",
				LastPublicIP: "1.1.1.1",
				UpdatedAt:    time.Now().Add(-time.Hour),
			},
		},
		reserveNext: 8,
	}
	resolver := &stubResolver{
		results: map[string][]string{
			"dl-007.example.com": {"1.1.1.1"},
		},
	}
	provider := &stubDNSProvider{enabled: true}
	svc := newDNSNodeService(repo, store, resolver, provider)

	item, err := svc.Report(context.Background(), newReportInput("2.2.2.2"))
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.DNSLabel != "dl-008" || item.DNSName != "dl-008.example.com" {
		t.Fatalf("expected rotated binding dl-008.example.com, got %s / %s", item.DNSLabel, item.DNSName)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(provider.calls))
	}
	if provider.calls[0].Name != "dl-008.example.com" {
		t.Fatalf("expected rotated provider name dl-008.example.com, got %s", provider.calls[0].Name)
	}
	if binding := store.bindings[testNodeUUID]; binding.DNSName != "dl-008.example.com" || binding.LastPublicIP != "2.2.2.2" {
		t.Fatalf("expected rotated local binding, got %+v", binding)
	}
}

func TestReportReassignsWhenLocalBindingMissingEvenIfDBHasOldBinding(t *testing.T) {
	initial := newReportInput("")
	initial.DNSLabel = "dl-010"
	initial.DNSName = "dl-010.example.com"
	repo := newDNSRepository(initial)
	store := &stubBindingStore{reserveNext: 11}
	resolver := &stubResolver{}
	provider := &stubDNSProvider{enabled: true}
	svc := newDNSNodeService(repo, store, resolver, provider)

	item, err := svc.Report(context.Background(), newReportInput("5.5.5.5"))
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if item.DNSLabel != "dl-011" || item.DNSName != "dl-011.example.com" {
		t.Fatalf("expected reassigned binding dl-011.example.com, got %s / %s", item.DNSLabel, item.DNSName)
	}
	if len(provider.calls) != 1 || provider.calls[0].Name != "dl-011.example.com" {
		t.Fatalf("unexpected provider calls: %+v", provider.calls)
	}
}

func TestReportReturnsErrorWhenLookupFails(t *testing.T) {
	initial := newReportInput("")
	initial.DNSLabel = "dl-007"
	initial.DNSName = "dl-007.example.com"
	repo := newDNSRepository(initial)
	store := &stubBindingStore{
		bindings: map[string]platformdns.Binding{
			testNodeUUID: {
				NodeUUID:     testNodeUUID,
				DNSLabel:     "dl-007",
				DNSName:      "dl-007.example.com",
				LastPublicIP: "1.1.1.1",
				UpdatedAt:    time.Now().Add(-time.Hour),
			},
		},
		reserveNext: 8,
	}
	resolver := &stubResolver{
		errs: map[string]error{
			"dl-007.example.com": context.DeadlineExceeded,
		},
	}
	provider := &stubDNSProvider{enabled: true}
	svc := newDNSNodeService(repo, store, resolver, provider)

	if _, err := svc.Report(context.Background(), newReportInput("2.2.2.2")); err == nil {
		t.Fatal("expected lookup error, got nil")
	}
	if len(provider.calls) != 0 {
		t.Fatalf("expected no provider call, got %d", len(provider.calls))
	}
	if len(store.saveCalls) != 0 {
		t.Fatalf("expected no local binding save, got %d", len(store.saveCalls))
	}
	if len(repo.saveDNSBindingCalls) != 0 {
		t.Fatalf("expected no dns binding mirror save, got %d", len(repo.saveDNSBindingCalls))
	}
}

func TestSyncDNSBindingsFromLocalMirrorsFileStateToDB(t *testing.T) {
	initial := newReportInput("")
	repo := newDNSRepository(initial)
	store := &stubBindingStore{
		listItems: []platformdns.Binding{
			{
				NodeUUID:     testNodeUUID,
				DNSLabel:     "dl-021",
				DNSName:      "dl-021.example.com",
				LastPublicIP: "7.7.7.7",
				UpdatedAt:    time.Now(),
			},
			{
				NodeUUID:     "missing-node",
				DNSLabel:     "dl-099",
				DNSName:      "dl-099.example.com",
				LastPublicIP: "9.9.9.9",
				UpdatedAt:    time.Now(),
			},
		},
	}
	svc := newDNSNodeService(repo, store, &stubResolver{}, &stubDNSProvider{enabled: true})

	if err := svc.SyncDNSBindingsFromLocal(context.Background()); err != nil {
		t.Fatalf("SyncDNSBindingsFromLocal returned error: %v", err)
	}
	if len(repo.saveDNSBindingCalls) != 1 {
		t.Fatalf("expected 1 dns binding mirror save, got %d", len(repo.saveDNSBindingCalls))
	}
	if repo.saveDNSBindingCalls[0].Label != "dl-021" {
		t.Fatalf("expected mirrored label dl-021, got %s", repo.saveDNSBindingCalls[0].Label)
	}
}
