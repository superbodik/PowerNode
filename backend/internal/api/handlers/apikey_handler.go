package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
)

type APIKeyHandler struct {
	DB *pgxpool.Pool
}

type apiKeySummary struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, name, permissions, last_used_at, created_at FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
		claims.UserID)
	if err != nil {
		http.Error(w, "failed to list api keys", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	keys := make([]apiKeySummary, 0)
	for rows.Next() {
		var k apiKeySummary
		var raw []byte
		if err := rows.Scan(&k.ID, &k.Name, &raw, &k.LastUsedAt, &k.CreatedAt); err != nil {
			http.Error(w, "failed to read api keys", http.StatusInternalServerError)
			return
		}
		k.Permissions = []string{}
		_ = json.Unmarshal(raw, &k.Permissions)
		keys = append(keys, k)
	}

	writeJSON(w, http.StatusOK, keys)
}

type createAPIKeyRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
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

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, "failed to generate key", http.StatusInternalServerError)
		return
	}
	token := "panel_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])

	var id int64
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO api_keys (user_id, name, token_hash, permissions) VALUES ($1, $2, $3, $4) RETURNING id`,
		claims.UserID, req.Name, tokenHash, permissionsJSON,
	).Scan(&id)
	if err != nil {
		http.Error(w, "failed to create api key", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": req.Name, "token": token})
}

func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if _, err := h.DB.Exec(r.Context(),
		`DELETE FROM api_keys WHERE id = $1 AND user_id = $2`, id, claims.UserID,
	); err != nil {
		http.Error(w, "failed to delete api key", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
