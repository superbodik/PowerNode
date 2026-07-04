package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/daemonclient"
	"github.com/yourorg/panel/internal/models"
)

type ServerHandler struct {
	DB         *pgxpool.Pool
	NodeClient func(nodeID int64) (*daemonclient.Client, error)
}

func (h *ServerHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT s.id, s.uuid, s.uuid_short, s.name, s.description, s.owner_id,
		       s.node_id, s.egg_id, s.docker_image, s.startup_command,
		       s.memory_mb, s.swap_mb, s.disk_mb, s.io_weight, s.cpu_percent,
		       s.allocation_limit, s.database_limit, s.backup_limit,
		       s.status, s.container_id, s.is_suspended, s.created_at, s.updated_at
		FROM servers s
		WHERE s.owner_id = $1 OR $2 = true
		ORDER BY s.created_at DESC`, claims.UserID, claims.IsAdmin)
	if err != nil {
		http.Error(w, "failed to list servers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	servers := make([]models.Server, 0)
	for rows.Next() {
		var s models.Server
		if err := rows.Scan(
			&s.ID, &s.UUID, &s.UUIDShort, &s.Name, &s.Description, &s.OwnerID,
			&s.NodeID, &s.EggID, &s.DockerImage, &s.StartupCommand,
			&s.MemoryMB, &s.SwapMB, &s.DiskMB, &s.IOWeight, &s.CPUPercent,
			&s.AllocationLimit, &s.DatabaseLimit, &s.BackupLimit,
			&s.Status, &s.ContainerID, &s.IsSuspended, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			http.Error(w, "failed to read servers", http.StatusInternalServerError)
			return
		}
		servers = append(servers, s)
	}

	writeJSON(w, http.StatusOK, servers)
}

func (h *ServerHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return
	}

	var s models.Server
	err = h.DB.QueryRow(r.Context(), `
		SELECT id, uuid, uuid_short, name, description, owner_id, node_id,
		       egg_id, docker_image, startup_command, memory_mb, swap_mb,
		       disk_mb, io_weight, cpu_percent, allocation_limit,
		       database_limit, backup_limit, status, container_id,
		       is_suspended, created_at, updated_at
		FROM servers WHERE uuid = $1`, id,
	).Scan(
		&s.ID, &s.UUID, &s.UUIDShort, &s.Name, &s.Description, &s.OwnerID,
		&s.NodeID, &s.EggID, &s.DockerImage, &s.StartupCommand,
		&s.MemoryMB, &s.SwapMB, &s.DiskMB, &s.IOWeight, &s.CPUPercent,
		&s.AllocationLimit, &s.DatabaseLimit, &s.BackupLimit,
		&s.Status, &s.ContainerID, &s.IsSuspended, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to load server", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, s)
}

type powerRequest struct {
	Action daemonclient.PowerAction `json:"action"`
}

func (h *ServerHandler) Power(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return
	}

	var req powerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var nodeID int64
	if err := h.DB.QueryRow(r.Context(),
		`SELECT node_id FROM servers WHERE uuid = $1`, id,
	).Scan(&nodeID); err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	client, err := h.NodeClient(nodeID)
	if err != nil {
		http.Error(w, "node unavailable", http.StatusBadGateway)
		return
	}

	resp, err := client.Power(r.Context(), id, req.Action)
	if err != nil {
		http.Error(w, "daemon rejected power action", http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
