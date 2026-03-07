package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"strings"
	"time"

	"github.com/SamuelFan1/Axis/internal/config"
	"github.com/SamuelFan1/Axis/internal/domain/node"
)

type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *nethttp.Client
}

type APIError struct {
	Error string `json:"error"`
}

type RegisterNodeRequest struct {
	UUID              string `json:"uuid,omitempty"`
	Hostname          string `json:"hostname"`
	ManagementAddress string `json:"management_address"`
	Region            string `json:"region"`
	Status            string `json:"status"`
}

type registerNodeResponse struct {
	Message string `json:"message"`
	Node    struct {
		UUID              string `json:"uuid"`
		Hostname          string `json:"hostname"`
		ManagementAddress string `json:"management_address"`
		Region            string `json:"region"`
		Status            string `json:"status"`
	} `json:"node"`
	Error string `json:"error"`
}

type listNodesResponse struct {
	Nodes []node.Node `json:"nodes"`
	Count int         `json:"count"`
	Error string      `json:"error"`
}

type getNodeResponse struct {
	Node  node.Node `json:"node"`
	Error string    `json:"error"`
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

type updateStatusResponse struct {
	Message string    `json:"message"`
	Node    node.Node `json:"node"`
	Error   string    `json:"error"`
}

type deleteNodeResponse struct {
	Message string `json:"message"`
	UUID    string `json:"uuid"`
	Error   string `json:"error"`
}

type listRegionsResponse struct {
	Regions []node.RegionSummary `json:"regions"`
	Count   int                  `json:"count"`
	Error   string               `json:"error"`
}

func NewClient(cfg config.CLIAuthConfig) *Client {
	return &Client{
		baseURL:  strings.TrimRight(cfg.APIURL, "/"),
		username: cfg.AdminUsername,
		password: cfg.AdminPassword,
		httpClient: &nethttp.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) RegisterNode(req RegisterNodeRequest) (node.Node, error) {
	var resp registerNodeResponse
	if err := c.doJSON(nethttp.MethodPost, "/api/v1/admin/nodes/register", req, &resp); err != nil {
		return node.Node{}, err
	}
	return node.Node{
		UUID:              resp.Node.UUID,
		Hostname:          resp.Node.Hostname,
		ManagementAddress: resp.Node.ManagementAddress,
		Region:            resp.Node.Region,
		Status:            resp.Node.Status,
	}, nil
}

func (c *Client) ListNodes() ([]node.Node, error) {
	var resp listNodesResponse
	if err := c.doJSON(nethttp.MethodGet, "/api/v1/nodes", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

func (c *Client) GetNode(uuid string) (node.Node, error) {
	var resp getNodeResponse
	if err := c.doJSON(nethttp.MethodGet, "/api/v1/nodes/"+uuid, nil, &resp); err != nil {
		return node.Node{}, err
	}
	return resp.Node, nil
}

func (c *Client) DeleteNode(uuid string) error {
	var resp deleteNodeResponse
	return c.doJSON(nethttp.MethodDelete, "/api/v1/nodes/"+uuid, nil, &resp)
}

func (c *Client) UpdateNodeStatus(uuid string, status string) (node.Node, error) {
	var resp updateStatusResponse
	if err := c.doJSON(nethttp.MethodPost, "/api/v1/nodes/"+uuid+"/status", updateStatusRequest{Status: status}, &resp); err != nil {
		return node.Node{}, err
	}
	return resp.Node, nil
}

func (c *Client) ListRegions() ([]node.RegionSummary, error) {
	var resp listRegionsResponse
	if err := c.doJSON(nethttp.MethodGet, "/api/v1/regions", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Regions, nil
}

func (c *Client) doJSON(method, path string, reqBody interface{}, out interface{}) error {
	var bodyReader *bytes.Reader
	if reqBody == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := nethttp.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode >= 400 {
		apiErr := APIError{}
		raw, _ := json.Marshal(out)
		_ = json.Unmarshal(raw, &apiErr)
		if apiErr.Error != "" {
			return fmt.Errorf(apiErr.Error)
		}
		return fmt.Errorf("axis api returned status %d", resp.StatusCode)
	}

	return nil
}
