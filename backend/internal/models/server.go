package models

import (
	"time"

	"github.com/google/uuid"
)

type ServerStatus string

const (
	StatusInstalling    ServerStatus = "installing"
	StatusInstallFailed ServerStatus = "install_failed"
	StatusSuspended     ServerStatus = "suspended"
	StatusOffline       ServerStatus = "offline"
	StatusStarting      ServerStatus = "starting"
	StatusRunning       ServerStatus = "running"
	StatusStopping      ServerStatus = "stopping"
)

type Server struct {
	ID             int64             `json:"id"`
	UUID           uuid.UUID         `json:"uuid"`
	UUIDShort      string            `json:"uuid_short"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	OwnerID        int64             `json:"owner_id"`
	NodeID         int64             `json:"node_id"`
	EggID          int               `json:"egg_id"`
	DockerImage    string            `json:"docker_image"`
	StartupCommand string            `json:"startup_command"`
	Environment    map[string]string `json:"environment"`

	MemoryMB      int64  `json:"memory_mb"`
	SwapMB        int64  `json:"swap_mb"`
	DiskMB        int64  `json:"disk_mb"`
	IOWeight      int    `json:"io_weight"`
	CPUPercent    *int   `json:"cpu_percent,omitempty"`
	ThreadsPinned string `json:"threads_pinned,omitempty"`

	AllocationLimit int `json:"allocation_limit"`
	DatabaseLimit   int `json:"database_limit"`
	BackupLimit     int `json:"backup_limit"`

	Status      ServerStatus `json:"status"`
	ContainerID string       `json:"container_id,omitempty"`
	IsSuspended bool         `json:"is_suspended"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ResourceStats struct {
	ServerUUID    uuid.UUID    `json:"server_uuid"`
	CPUPercent    float64      `json:"cpu_percent"`
	MemoryBytes   int64        `json:"memory_bytes"`
	DiskBytes     int64        `json:"disk_bytes"`
	NetworkRx     int64        `json:"network_rx"`
	NetworkTx     int64        `json:"network_tx"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	State         ServerStatus `json:"state"`
}

type Egg struct {
	ID             int       `json:"id"`
	UUID           uuid.UUID `json:"uuid"`
	Category       string    `json:"category"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	DockerImage    string    `json:"docker_image"`
	StartupCommand string    `json:"startup_command"`
	StopCommand    string    `json:"stop_command"`
}

type EggVariable struct {
	ID           int64  `json:"id"`
	EggID        int    `json:"egg_id"`
	Name         string `json:"name"`
	EnvVariable  string `json:"env_variable"`
	DefaultValue string `json:"default_value"`
	IsEditable   bool   `json:"is_editable"`
	Rules        string `json:"rules"`
}
