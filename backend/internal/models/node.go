package models

import (
	"time"

	"github.com/google/uuid"
)

type Node struct {
	ID                 int64      `json:"id"`
	UUID               uuid.UUID  `json:"uuid"`
	Name               string     `json:"name"`
	LocationID         int        `json:"location_id"`
	FQDN               string     `json:"fqdn"`
	Scheme             string     `json:"scheme"`
	DaemonPort         int        `json:"daemon_port"`
	DaemonTokenHash    string     `json:"-"`
	MemoryMB           int64      `json:"memory_mb"`
	MemoryOverallocate int        `json:"memory_overallocate"`
	DiskMB             int64      `json:"disk_mb"`
	DiskOverallocate   int        `json:"disk_overallocate"`
	CPUPercent         *int       `json:"cpu_percent,omitempty"`
	IsPublic           bool       `json:"is_public"`
	MaintenanceMode    bool       `json:"maintenance_mode"`
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
	AgentVersion       string     `json:"agent_version"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (n Node) IsOnline() bool {
	return n.LastSeenAt != nil && time.Since(*n.LastSeenAt) < 30*time.Second
}

type Allocation struct {
	ID       int64  `json:"id"`
	NodeID   int64  `json:"node_id"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Alias    string `json:"alias,omitempty"`
	ServerID *int64 `json:"server_id,omitempty"`
}
