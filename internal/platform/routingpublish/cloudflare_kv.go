package routingpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/routing"
)

const cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"

type CloudflareKVPublisher struct {
	accountID   string
	namespaceID string
	apiToken    string
	httpClient  *http.Client
}

type kvBulkItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type bulkUpdateEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		SuccessfulKeyCount int      `json:"successful_key_count"`
		UnsuccessfulKeys   []string `json:"unsuccessful_keys"`
	} `json:"result"`
}

func NewCloudflareKVPublisher(cfg config.RoutingConfig) Publisher {
	return &CloudflareKVPublisher{
		accountID:   strings.TrimSpace(cfg.CloudflareAccountID),
		namespaceID: strings.TrimSpace(cfg.CloudflareKVNamespaceID),
		apiToken:    strings.TrimSpace(cfg.CloudflareAPIToken),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (p *CloudflareKVPublisher) Enabled() bool {
	return p != nil && p.accountID != "" && p.namespaceID != "" && p.apiToken != ""
}

func (p *CloudflareKVPublisher) PublishSnapshot(ctx context.Context, manifest routing.Manifest, bundles []routing.Bundle) error {
	if !p.Enabled() {
		return fmt.Errorf("cloudflare kv publisher is not configured")
	}

	items := make([]kvBulkItem, 0, len(bundles)+1)

	manifestPayload, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal routing manifest for kv: %w", err)
	}
	items = append(items, kvBulkItem{
		Key:   routing.ManifestKVKey,
		Value: string(manifestPayload),
	})

	for _, bundle := range bundles {
		payload, err := json.Marshal(bundle)
		if err != nil {
			return fmt.Errorf("marshal routing bundle for kv: %w", err)
		}
		items = append(items, kvBulkItem{
			Key:   bundle.Key,
			Value: string(payload),
		})
	}

	body, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("marshal kv bulk payload: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		fmt.Sprintf(
			"%s/accounts/%s/storage/kv/namespaces/%s/bulk",
			cloudflareAPIBaseURL,
			p.accountID,
			p.namespaceID,
		),
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build cloudflare kv bulk request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send cloudflare kv bulk request: %w", err)
	}
	defer resp.Body.Close()

	var envelope bulkUpdateEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode cloudflare kv bulk response: %w", err)
	}
	if !envelope.Success {
		return fmt.Errorf("cloudflare kv bulk update failed: %s", joinErrors(envelope.Errors))
	}
	if len(envelope.Result.UnsuccessfulKeys) > 0 {
		return fmt.Errorf("cloudflare kv bulk update partially failed: %s", strings.Join(envelope.Result.UnsuccessfulKeys, ", "))
	}

	return nil
}

func joinErrors(items []struct {
	Message string `json:"message"`
}) string {
	if len(items) == 0 {
		return "unknown error"
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Message) != "" {
			parts = append(parts, strings.TrimSpace(item.Message))
		}
	}
	if len(parts) == 0 {
		return "unknown error"
	}
	return strings.Join(parts, "; ")
}
