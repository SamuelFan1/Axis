package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type RegionalSnapshotPublisherClient struct {
	baseURL    string
	token      string
	httpClient *nethttp.Client
}

func NewRegionalSnapshotPublisherClient(cfg config.AggregationConfig) *RegionalSnapshotPublisherClient {
	return &RegionalSnapshotPublisherClient{
		baseURL: strings.TrimRight(cfg.CenterAPIURL, "/"),
		token:   cfg.SharedToken,
		httpClient: &nethttp.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *RegionalSnapshotPublisherClient) Enabled() bool {
	return c != nil && c.baseURL != "" && c.token != ""
}

func (c *RegionalSnapshotPublisherClient) PublishRegionalSnapshot(ctx context.Context, snapshot node.RegionalNodeStatusSnapshot) error {
	if !c.Enabled() {
		return fmt.Errorf("regional snapshot publisher is not configured")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal regional snapshot: %w", err)
	}
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, c.baseURL+"/api/v1/internal/aggregation/regional-snapshots", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build regional snapshot request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Axis-Internal-Token", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send regional snapshot request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var apiErr APIError
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error != "" {
			return fmt.Errorf("regional snapshot request failed: %s", apiErr.Error)
		}
		return fmt.Errorf("regional snapshot request failed with status %d", resp.StatusCode)
	}
	return nil
}
