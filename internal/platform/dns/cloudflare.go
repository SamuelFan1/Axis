package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
)

const cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"

type CloudflareProvider struct {
	zone       string
	apiToken   string
	httpClient *http.Client

	mu     sync.Mutex
	zoneID string
}

type cloudflareEnvelope[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result     T `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalPages int `json:"total_pages"`
		Count      int `json:"count"`
		TotalCount int `json:"total_count"`
	} `json:"result_info"`
}

type cloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cloudflareDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

func NewCloudflareProvider(cfg config.DNSConfig) Provider {
	return NewCloudflareProviderForZone(cfg, cfg.Zone)
}

func NewCloudflareProviderForZone(cfg config.DNSConfig, zone string) Provider {
	return &CloudflareProvider{
		zone:     strings.Trim(strings.TrimSpace(zone), "."),
		apiToken: strings.TrimSpace(cfg.CloudflareAPIToken),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *CloudflareProvider) Enabled() bool {
	return true
}

func (p *CloudflareProvider) MaxSequence(ctx context.Context, prefix string) (int, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return 0, fmt.Errorf("dns prefix is required")
	}

	zoneID, err := p.getZoneID(ctx)
	if err != nil {
		return 0, err
	}

	maxSequence := 0
	page := 1
	for {
		values := url.Values{}
		values.Set("type", "A")
		values.Set("page", strconv.Itoa(page))
		values.Set("per_page", "100")

		var resp cloudflareEnvelope[[]cloudflareDNSRecord]
		if err := p.doJSON(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records?"+values.Encode(), nil, &resp); err != nil {
			return 0, fmt.Errorf("list cloudflare dns records for max sequence: %w", err)
		}

		if len(resp.Result) == 0 {
			break
		}

		for _, record := range resp.Result {
			label := strings.TrimSpace(record.Name)
			if strings.HasSuffix(strings.ToLower(label), "."+strings.ToLower(p.zone)) {
				label = strings.TrimSuffix(label, "."+p.zone)
			}
			sequence, ok := ParseDNSSequence(prefix, label)
			if ok && sequence > maxSequence {
				maxSequence = sequence
			}
		}

		if resp.ResultInfo.TotalPages > 0 {
			if page >= resp.ResultInfo.TotalPages {
				break
			}
		} else if len(resp.Result) < 100 {
			break
		}
		page++
	}

	return maxSequence, nil
}

func (p *CloudflareProvider) EnsureRecord(ctx context.Context, record Record) error {
	if strings.ToUpper(strings.TrimSpace(record.Type)) != "A" {
		return fmt.Errorf("cloudflare provider only supports A records")
	}
	if strings.TrimSpace(record.Name) == "" {
		return fmt.Errorf("dns record name is required")
	}
	if strings.TrimSpace(record.Content) == "" {
		return fmt.Errorf("dns record content is required")
	}

	zoneID, err := p.getZoneID(ctx)
	if err != nil {
		return err
	}

	existing, err := p.findDNSRecord(ctx, zoneID, record.Name, "A")
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.Content == record.Content && existing.TTL == record.TTL && existing.Proxied == record.Proxied {
			return nil
		}
		return p.updateDNSRecord(ctx, zoneID, existing.ID, record)
	}

	return p.createDNSRecord(ctx, zoneID, record)
}

func (p *CloudflareProvider) ListRecords(ctx context.Context, recordType string) ([]ManagedRecord, error) {
	recordType = strings.ToUpper(strings.TrimSpace(recordType))
	if recordType == "" {
		recordType = "A"
	}
	zoneID, err := p.getZoneID(ctx)
	if err != nil {
		return nil, err
	}

	var records []ManagedRecord
	page := 1
	for {
		values := url.Values{}
		values.Set("type", recordType)
		values.Set("page", strconv.Itoa(page))
		values.Set("per_page", "100")

		var resp cloudflareEnvelope[[]cloudflareDNSRecord]
		if err := p.doJSON(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records?"+values.Encode(), nil, &resp); err != nil {
			return nil, fmt.Errorf("list cloudflare dns records: %w", err)
		}
		for _, item := range resp.Result {
			records = append(records, ManagedRecord{
				ID:      item.ID,
				Name:    item.Name,
				Type:    item.Type,
				Content: item.Content,
				TTL:     item.TTL,
				Proxied: item.Proxied,
			})
		}

		if resp.ResultInfo.TotalPages > 0 {
			if page >= resp.ResultInfo.TotalPages {
				break
			}
		} else if len(resp.Result) < 100 {
			break
		}
		page++
	}
	return records, nil
}

func (p *CloudflareProvider) CreateRecord(ctx context.Context, record Record) error {
	zoneID, err := p.getZoneID(ctx)
	if err != nil {
		return err
	}
	return p.createDNSRecord(ctx, zoneID, record)
}

func (p *CloudflareProvider) UpdateRecord(ctx context.Context, id string, record Record) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("dns record id is required")
	}
	zoneID, err := p.getZoneID(ctx)
	if err != nil {
		return err
	}
	return p.updateDNSRecord(ctx, zoneID, id, record)
}

func (p *CloudflareProvider) DeleteRecord(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("dns record id is required")
	}
	zoneID, err := p.getZoneID(ctx)
	if err != nil {
		return err
	}
	var resp cloudflareEnvelope[cloudflareDNSRecord]
	if err := p.doJSON(ctx, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+id, nil, &resp); err != nil {
		return fmt.Errorf("delete cloudflare dns record: %w", err)
	}
	return nil
}

func (p *CloudflareProvider) getZoneID(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.zoneID != "" {
		defer p.mu.Unlock()
		return p.zoneID, nil
	}
	p.mu.Unlock()

	values := url.Values{}
	values.Set("name", p.zone)

	var resp cloudflareEnvelope[[]cloudflareZone]
	if err := p.doJSON(ctx, http.MethodGet, "/zones?"+values.Encode(), nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Result) == 0 {
		return "", fmt.Errorf("cloudflare zone %s not found", p.zone)
	}

	p.mu.Lock()
	p.zoneID = resp.Result[0].ID
	p.mu.Unlock()
	return resp.Result[0].ID, nil
}

func (p *CloudflareProvider) findDNSRecord(ctx context.Context, zoneID, name, recordType string) (*cloudflareDNSRecord, error) {
	values := url.Values{}
	values.Set("type", recordType)
	values.Set("name", name)

	var resp cloudflareEnvelope[[]cloudflareDNSRecord]
	if err := p.doJSON(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records?"+values.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	for _, item := range resp.Result {
		if item.Name == name && item.Type == recordType {
			record := item
			return &record, nil
		}
	}
	return nil, nil
}

func (p *CloudflareProvider) createDNSRecord(ctx context.Context, zoneID string, record Record) error {
	payload := map[string]interface{}{
		"type":    "A",
		"name":    record.Name,
		"content": record.Content,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}
	var resp cloudflareEnvelope[cloudflareDNSRecord]
	if err := p.doJSON(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", payload, &resp); err != nil {
		return fmt.Errorf("create cloudflare dns record: %w", err)
	}
	return nil
}

func (p *CloudflareProvider) updateDNSRecord(ctx context.Context, zoneID, recordID string, record Record) error {
	payload := map[string]interface{}{
		"type":    "A",
		"name":    record.Name,
		"content": record.Content,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}
	var resp cloudflareEnvelope[cloudflareDNSRecord]
	if err := p.doJSON(ctx, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+recordID, payload, &resp); err != nil {
		return fmt.Errorf("update cloudflare dns record: %w", err)
	}
	return nil
}

func (p *CloudflareProvider) doJSON(ctx context.Context, method, path string, payload interface{}, out interface{}) error {
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return fmt.Errorf("encode cloudflare request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, cloudflareAPIBaseURL+path, &body)
	if err != nil {
		return fmt.Errorf("build cloudflare request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send cloudflare request: %w", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode cloudflare response: %w", err)
	}

	switch typed := out.(type) {
	case *cloudflareEnvelope[[]cloudflareZone]:
		if !typed.Success {
			return fmt.Errorf("cloudflare api: %s", joinCloudflareErrors(typed.Errors))
		}
	case *cloudflareEnvelope[[]cloudflareDNSRecord]:
		if !typed.Success {
			return fmt.Errorf("cloudflare api: %s", joinCloudflareErrors(typed.Errors))
		}
	case *cloudflareEnvelope[cloudflareDNSRecord]:
		if !typed.Success {
			return fmt.Errorf("cloudflare api: %s", joinCloudflareErrors(typed.Errors))
		}
	}

	return nil
}

func joinCloudflareErrors(items []struct {
	Message string `json:"message"`
}) string {
	if len(items) == 0 {
		return "unknown error"
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Message) == "" {
			continue
		}
		parts = append(parts, item.Message)
	}
	if len(parts) == 0 {
		return "unknown error"
	}
	return strings.Join(parts, "; ")
}
