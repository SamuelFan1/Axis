package workeradmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
)

const manualNodeStatusPath = "/worker/manual-node-status"

type Client interface {
	Enabled() bool
	DisableNode(ctx context.Context, originLabel string) error
	EnableNode(ctx context.Context, originLabel string) error
	GetManualNodeStatuses(ctx context.Context, originLabels []string) (map[string]string, error)
}

type HTTPClient struct {
	baseURL     string
	adminSecret string
	httpClient  *http.Client
}

type statusRequest struct {
	Node    string                 `json:"node"`
	Status  string                 `json:"status"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type statusResponse struct {
	OK                  bool   `json:"ok"`
	Node                string `json:"node"`
	Status              string `json:"status"`
	ManualBlacklistSize int    `json:"manual_blacklist_size"`
	Error               string `json:"error"`
}

type bulkStatusResponse struct {
	OK    bool              `json:"ok"`
	Nodes map[string]string `json:"nodes"`
	Count int               `json:"count"`
	Error string            `json:"error"`
}

func NewClient(cfg config.WorkerAdminConfig) Client {
	return &HTTPClient{
		baseURL:     strings.TrimRight(strings.TrimSpace(cfg.WorkerURL), "/"),
		adminSecret: strings.TrimSpace(cfg.WorkerAdminSecret),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *HTTPClient) Enabled() bool {
	return c != nil && c.baseURL != "" && c.adminSecret != ""
}

func (c *HTTPClient) DisableNode(ctx context.Context, originLabel string) error {
	return c.setManualNodeStatus(ctx, originLabel, "disabled")
}

func (c *HTTPClient) EnableNode(ctx context.Context, originLabel string) error {
	return c.setManualNodeStatus(ctx, originLabel, "enabled")
}

func (c *HTTPClient) GetManualNodeStatuses(ctx context.Context, originLabels []string) (map[string]string, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("worker admin is not configured")
	}

	nodes := make([]string, 0, len(originLabels))
	for _, originLabel := range originLabels {
		normalized := strings.ToLower(strings.TrimSpace(originLabel))
		if normalized == "" {
			continue
		}
		nodes = append(nodes, normalized)
	}
	if len(nodes) == 0 {
		return map[string]string{}, nil
	}

	values := url.Values{}
	values.Set("nodes", strings.Join(nodes, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+manualNodeStatusPath+"?"+values.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build worker admin request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.adminSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send worker admin request: %w", err)
	}
	defer resp.Body.Close()

	var body bulkStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode worker admin response: %w", err)
	}
	if resp.StatusCode >= 400 {
		if strings.TrimSpace(body.Error) != "" {
			return nil, fmt.Errorf("worker admin request failed: %s", strings.TrimSpace(body.Error))
		}
		return nil, fmt.Errorf("worker admin request failed with status %d", resp.StatusCode)
	}
	if !body.OK {
		return nil, fmt.Errorf("worker admin request failed")
	}
	if body.Nodes == nil {
		return map[string]string{}, nil
	}
	return body.Nodes, nil
}

func (c *HTTPClient) setManualNodeStatus(ctx context.Context, originLabel string, status string) error {
	if !c.Enabled() {
		return fmt.Errorf("worker admin is not configured")
	}
	originLabel = strings.ToLower(strings.TrimSpace(originLabel))
	if originLabel == "" {
		return fmt.Errorf("origin label is required")
	}

	payload, err := json.Marshal(statusRequest{
		Node:   originLabel,
		Status: status,
		Details: map[string]interface{}{
			"source": "axis-service-status",
		},
	})
	if err != nil {
		return fmt.Errorf("marshal worker admin request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+manualNodeStatusPath, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build worker admin request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.adminSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send worker admin request: %w", err)
	}
	defer resp.Body.Close()

	var body statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("decode worker admin response: %w", err)
	}
	if resp.StatusCode >= 400 {
		if strings.TrimSpace(body.Error) != "" {
			return fmt.Errorf("worker admin request failed: %s", strings.TrimSpace(body.Error))
		}
		return fmt.Errorf("worker admin request failed with status %d", resp.StatusCode)
	}
	if !body.OK {
		return fmt.Errorf("worker admin request failed")
	}
	return nil
}
