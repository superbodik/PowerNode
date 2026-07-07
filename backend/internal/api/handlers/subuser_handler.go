package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
)

type SubuserHandler struct {
	DB *pgxpool.Pool
}

type subuserSummary struct {
	ID          int64    `json:"id"`
	UserID      int64    `json:"user_id"`
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
}

func (h *SubuserHandler) resolveOwnedServer(w http.ResponseWriter, r *http.Request) (int64, bool) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}

	var serverID, ownerID int64
	if err := h.DB.QueryRow(r.Context(),
		`SELECT id, owner_id FROM servers WHERE uuid = $1`, chi.URLParam(r, "uuid"),
	).Scan(&serverID, &ownerID); err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return 0, false
	}
	if !claims.IsAdmin && claims.UserID != ownerID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return 0, false
	}

	return serverID, true
}

func (h *SubuserHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.resolveOwnedServer(w, r)
	if !ok {
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT su.id, su.user_id, u.email, su.permissions
		FROM server_subusers su JOIN users u ON u.id = su.user_id
		WHERE su.server_id = $1
		ORDER BY su.created_at`, serverID)
	if err != nil {
		http.Error(w, "failed to list subusers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	subusers := make([]subuserSummary, 0)
	for rows.Next() {
		var s subuserSummary
		var raw []byte
		if err := rows.Scan(&s.ID, &s.UserID, &s.Email, &raw); err != nil {
			http.Error(w, "failed to read subusers", http.StatusInternalServerError)
			return
		}
		s.Permissions = []string{}
		_ = json.Unmarshal(raw, &s.Permissions)
		subusers = append(subusers, s)
	}

	writeJSON(w, http.StatusOK, subusers)
}

type createSubuserRequest struct {
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
}

func (h *SubuserHandler) Create(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.resolveOwnedServer(w, r)
	if !ok {
		return
	}

	var req createSubuserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}
	if req.Permissions == nil {
		req.Permissions = []string{}
	}
	for _, p := range req.Permissions {
		if !auth.IsValidPermission(p) {
			http.Error(w, "unknown permission: "+p, http.StatusBadRequest)
			return
		}
	}

	var userID int64
	if err := h.DB.QueryRow(r.Context(),
		`SELECT id FROM users WHERE email = $1`, req.Email,
	).Scan(&userID); err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "no user with that email", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to look up user", http.StatusInternalServerError)
		return
	}

	permissionsJSON, err := json.Marshal(req.Permissions)
	if err != nil {
		http.Error(w, "invalid permissions", http.StatusBadRequest)
		return
	}

	var id int64
	if err := h.DB.QueryRow(r.Context(), `
		INSERT INTO server_subusers (server_id, user_id, permissions)
		VALUES ($1, $2, $3)
		RETURNING id`,
		serverID, userID, permissionsJSON,
	).Scan(&id); err != nil {
		http.Error(w, "failed to add subuser (already added?)", http.StatusConflict)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

type updateSubuserRequest struct {
	Permissions []string `json:"permissions"`
}

func (h *SubuserHandler) Update(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.resolveOwnedServer(w, r)
	if !ok {
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req updateSubuserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Permissions == nil {
		req.Permissions = []string{}
	}
	for _, p := range req.Permissions {
		if !auth.IsValidPermission(p) {
			http.Error(w, "unknown permission: "+p, http.StatusBadRequest)
			return
		}
	}

	permissionsJSON, err := json.Marshal(req.Permissions)
	if err != nil {
		http.Error(w, "invalid permissions", http.StatusBadRequest)
		return
	}

	if _, err := h.DB.Exec(r.Context(),
		`UPDATE server_subusers SET permissions = $1 WHERE id = $2 AND server_id = $3`,
		permissionsJSON, id, serverID,
	); err != nil {
		http.Error(w, "failed to update subuser", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SubuserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.resolveOwnedServer(w, r)
	if !ok {
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if _, err := h.DB.Exec(r.Context(),
		`DELETE FROM server_subusers WHERE id = $1 AND server_id = $2`, id, serverID,
	); err != nil {
		http.Error(w, "failed to remove subuser", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
