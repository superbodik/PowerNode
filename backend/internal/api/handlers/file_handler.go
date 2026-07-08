package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/daemonclient"
)

type FileHandler struct {
	DB         *pgxpool.Pool
	NodeClient func(nodeID int64) (*daemonclient.Client, error)
	Subusers   *auth.SubuserChecker
}

func (h *FileHandler) resolve(w http.ResponseWriter, r *http.Request, permission string) (uuid.UUID, *daemonclient.Client, bool) {
	serverUUID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return uuid.UUID{}, nil, false
	}

	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return uuid.UUID{}, nil, false
	}

	var serverID, nodeID, ownerID int64
	if err := h.DB.QueryRow(r.Context(),
		`SELECT id, node_id, owner_id FROM servers WHERE uuid = $1`, serverUUID,
	).Scan(&serverID, &nodeID, &ownerID); err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return uuid.UUID{}, nil, false
	}
	if !claims.HasKeyPermission(permission) || !h.Subusers.CanAccessServer(r.Context(), claims, ownerID, serverID, permission) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.UUID{}, nil, false
	}

	client, err := h.NodeClient(nodeID)
	if err != nil {
		http.Error(w, "node unavailable", http.StatusBadGateway)
		return uuid.UUID{}, nil, false
	}

	return serverUUID, client, true
}

func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	serverUUID, client, ok := h.resolve(w, r, auth.PermFilesRead)
	if !ok {
		return
	}

	entries, err := client.ListFiles(r.Context(), serverUUID, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "failed to list files: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, entries)
}

func (h *FileHandler) Read(w http.ResponseWriter, r *http.Request) {
	serverUUID, client, ok := h.resolve(w, r, auth.PermFilesRead)
	if !ok {
		return
	}

	content, mtime, err := client.ReadFile(r.Context(), serverUUID, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "failed to read file: "+err.Error(), http.StatusBadGateway)
		return
	}

	if mtime > 0 {
		w.Header().Set("X-File-Mtime", strconv.FormatInt(mtime, 10))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(content)
}

func (h *FileHandler) Write(w http.ResponseWriter, r *http.Request) {
	serverUUID, client, ok := h.resolve(w, r, auth.PermFilesWrite)
	if !ok {
		return
	}

	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "file too large or failed to read request body", http.StatusBadRequest)
		return
	}

	var expectedMtime int64
	if raw := r.Header.Get("X-Expected-Mtime"); raw != "" {
		expectedMtime, _ = strconv.ParseInt(raw, 10, 64)
	}

	err = client.WriteFile(r.Context(), serverUUID, r.URL.Query().Get("path"), content, expectedMtime)
	if errors.Is(err, daemonclient.ErrFileConflict) {
		http.Error(w, "this file was changed on disk since you opened it — reload it and re-apply your edits", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "failed to write file: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	serverUUID, client, ok := h.resolve(w, r, auth.PermFilesWrite)
	if !ok {
		return
	}

	if err := client.DeleteFile(r.Context(), serverUUID, r.URL.Query().Get("path")); err != nil {
		http.Error(w, "failed to delete file: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *FileHandler) CreateDirectory(w http.ResponseWriter, r *http.Request) {
	serverUUID, client, ok := h.resolve(w, r, auth.PermFilesWrite)
	if !ok {
		return
	}

	if err := client.CreateDirectory(r.Context(), serverUUID, r.URL.Query().Get("path")); err != nil {
		http.Error(w, "failed to create directory: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type renameFileRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (h *FileHandler) Rename(w http.ResponseWriter, r *http.Request) {
	serverUUID, client, ok := h.resolve(w, r, auth.PermFilesWrite)
	if !ok {
		return
	}

	var req renameFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := client.RenameFile(r.Context(), serverUUID, req.From, req.To); err != nil {
		http.Error(w, "failed to rename file: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
