package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
)

type AllocationHandler struct {
	DB *pgxpool.Pool
}

type allocationSummary struct {
	ID       int64   `json:"id"`
	NodeID   int64   `json:"node_id"`
	IP       string  `json:"ip"`
	Port     int     `json:"port"`
	Alias    *string `json:"alias"`
	ServerID *int64  `json:"server_id"`
}

func (h *AllocationHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	nodeIDStr := r.URL.Query().Get("node_id")
	nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64)
	if err != nil {
		http.Error(w, "node_id query param is required", http.StatusBadRequest)
		return
	}

	if !claims.IsAdmin {
		var isPublic bool
		if err := h.DB.QueryRow(r.Context(),
			`SELECT is_public FROM nodes WHERE id = $1`, nodeID,
		).Scan(&isPublic); err != nil || !isPublic {
			http.Error(w, "node not found", http.StatusNotFound)
			return
		}
	}

	freeOnly := r.URL.Query().Get("free") == "true"

	query := `SELECT id, node_id, HOST(ip), port, alias, server_id FROM allocations WHERE node_id = $1`
	if freeOnly {
		query += ` AND server_id IS NULL`
	}
	query += ` ORDER BY port`

	rows, err := h.DB.Query(r.Context(), query, nodeID)
	if err != nil {
		http.Error(w, "failed to list allocations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	allocations := make([]allocationSummary, 0)
	for rows.Next() {
		var a allocationSummary
		if err := rows.Scan(&a.ID, &a.NodeID, &a.IP, &a.Port, &a.Alias, &a.ServerID); err != nil {
			http.Error(w, "failed to read allocations", http.StatusInternalServerError)
			return
		}
		allocations = append(allocations, a)
	}

	writeJSON(w, http.StatusOK, allocations)
}

type createAllocationRequest struct {
	NodeID  int64  `json:"node_id"`
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	PortEnd int    `json:"port_end"`
	Alias   string `json:"alias"`
}

const maxAllocationRangeSize = 1000

func (h *AllocationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.NodeID == 0 || req.IP == "" || req.Port == 0 {
		http.Error(w, "node_id, ip and port are required", http.StatusBadRequest)
		return
	}

	portEnd := req.PortEnd
	if portEnd == 0 {
		portEnd = req.Port
	}
	if portEnd < req.Port {
		http.Error(w, "port_end must be greater than or equal to port", http.StatusBadRequest)
		return
	}
	if portEnd-req.Port+1 > maxAllocationRangeSize {
		http.Error(w, "port range too large (max 1000 ports per request)", http.StatusBadRequest)
		return
	}

	var alias *string
	if req.Alias != "" {
		alias = &req.Alias
	}

	ctx := r.Context()
	tx, err := h.DB.Begin(ctx)
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	created := 0
	for port := req.Port; port <= portEnd; port++ {
		tag, err := tx.Exec(ctx, `
			INSERT INTO allocations (node_id, ip, port, alias)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (node_id, ip, port) DO NOTHING`,
			req.NodeID, req.IP, port, alias,
		)
		if err != nil {
			http.Error(w, "failed to create allocations", http.StatusInternalServerError)
			return
		}
		created += int(tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "failed to commit allocations", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"created": created})
}

func (h *AllocationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid allocation id", http.StatusBadRequest)
		return
	}

	res, err := h.DB.Exec(r.Context(),
		`DELETE FROM allocations WHERE id = $1 AND server_id IS NULL`, id)
	if err != nil {
		http.Error(w, "failed to delete allocation", http.StatusInternalServerError)
		return
	}
	if res.RowsAffected() == 0 {
		http.Error(w, "allocation not found or still in use by a server", http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
