package daemonclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	baseURL     string
	daemonToken string
	http        *http.Client
}

func New(baseURL, daemonToken string) *Client {
	return &Client{
		baseURL:     baseURL,
		daemonToken: daemonToken,
		http:        &http.Client{Timeout: 15 * time.Second},
	}
}

type PowerAction string

const (
	PowerStart   PowerAction = "start"
	PowerStop    PowerAction = "stop"
	PowerRestart PowerAction = "restart"
	PowerKill    PowerAction = "kill"
)

type CreateServerRequest struct {
	ServerUUID     uuid.UUID         `json:"server_uuid"`
	DockerImage    string            `json:"docker_image"`
	StartupCommand string            `json:"startup_command"`
	Environment    map[string]string `json:"environment"`
	MemoryMB       int64             `json:"memory_mb"`
	SwapMB         int64             `json:"swap_mb"`
	DiskMB         int64             `json:"disk_mb"`
	IOWeight       int               `json:"io_weight"`
	CPUPercent     int               `json:"cpu_percent"`
	InstallScript  string            `json:"install_script"`
}

type OperationResponse struct {
	ServerUUID uuid.UUID `json:"server_uuid"`
	Success    bool      `json:"success"`
	Message    string    `json:"message"`
	State      string    `json:"state"`
}

func (c *Client) CreateServer(ctx context.Context, req CreateServerRequest) (*OperationResponse, error) {
	var resp OperationResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/servers", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Power(ctx context.Context, serverUUID uuid.UUID, action PowerAction) (*OperationResponse, error) {
	var resp OperationResponse
	path := fmt.Sprintf("/api/v1/servers/%s/power", serverUUID)
	if err := c.doJSON(ctx, http.MethodPost, path, map[string]string{"action": string(action)}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) DeleteServer(ctx context.Context, serverUUID uuid.UUID) error {
	path := fmt.Sprintf("/api/v1/servers/%s", serverUUID)
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

type ResourceStats struct {
	ServerUUID    uuid.UUID `json:"server_uuid"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryBytes   int64     `json:"memory_bytes"`
	DiskBytes     int64     `json:"disk_bytes"`
	NetworkRx     int64     `json:"network_rx"`
	NetworkTx     int64     `json:"network_tx"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	State         string    `json:"state"`
}

func (c *Client) Stats(ctx context.Context, serverUUID uuid.UUID) (*ResourceStats, error) {
	var resp ResourceStats
	path := fmt.Sprintf("/api/v1/servers/%s/stats", serverUUID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) DialConsole(ctx context.Context, serverUUID uuid.UUID) (*websocket.Conn, error) {
	wsURL := strings.Replace(c.baseURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	url := fmt.Sprintf("%s/ws/servers/%s", wsURL, serverUUID)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.daemonToken)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, header)
	if err != nil {
		return nil, fmt.Errorf("dial daemon console: %w", err)
	}
	return conn, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, out interface{}) error {
	var reader *bytes.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(buf)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.daemonToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call node daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("node daemon returned %d", resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
