package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
)

type NodeHandler struct {
	DB *pgxpool.Pool
}

type createNodeRequest struct {
	Name               string `json:"name"`
	LocationID         int    `json:"location_id"`
	FQDN               string `json:"fqdn"`
	Scheme             string `json:"scheme"`
	DaemonPort         int    `json:"daemon_port"`
	MemoryMB           int64  `json:"memory_mb"`
	DiskMB             int64  `json:"disk_mb"`
}

type createNodeResponse struct {
	ID          int64  `json:"id"`
	DaemonToken string `json:"daemon_token"`
}

func (h *NodeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Scheme == "" {
		req.Scheme = "https"
	}
	if req.DaemonPort == 0 {
		req.DaemonPort = 8443
	}

	rawToken, err := generateToken(32)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	tokenHash, err := auth.HashPassword(rawToken)
	if err != nil {
		http.Error(w, "failed to hash token", http.StatusInternalServerError)
		return
	}

	var id int64
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO nodes (name, location_id, fqdn, scheme, daemon_port,
		                    daemon_token_hash, memory_mb, disk_mb)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		req.Name, req.LocationID, req.FQDN, req.Scheme, req.DaemonPort,
		tokenHash, req.MemoryMB, req.DiskMB,
	).Scan(&id)
	if err != nil {
		http.Error(w, "failed to create node", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, createNodeResponse{ID: id, DaemonToken: rawToken})
}

func (h *NodeHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, name, fqdn, scheme, daemon_port, memory_mb, disk_mb,
		       maintenance_mode, last_seen_at
		FROM nodes ORDER BY name`)
	if err != nil {
		http.Error(w, "failed to list nodes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type nodeSummary struct {
		ID              int64   `json:"id"`
		Name            string  `json:"name"`
		FQDN            string  `json:"fqdn"`
		Scheme          string  `json:"scheme"`
		DaemonPort      int     `json:"daemon_port"`
		MemoryMB        int64   `json:"memory_mb"`
		DiskMB          int64   `json:"disk_mb"`
		MaintenanceMode bool    `json:"maintenance_mode"`
		LastSeenAt      *string `json:"last_seen_at"`
	}

	nodes := make([]nodeSummary, 0)
	for rows.Next() {
		var n nodeSummary
		if err := rows.Scan(&n.ID, &n.Name, &n.FQDN, &n.Scheme, &n.DaemonPort,
			&n.MemoryMB, &n.DiskMB, &n.MaintenanceMode, &n.LastSeenAt); err != nil {
			http.Error(w, "failed to read nodes", http.StatusInternalServerError)
			return
		}
		nodes = append(nodes, n)
	}

	writeJSON(w, http.StatusOK, nodes)
}

func generateToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
