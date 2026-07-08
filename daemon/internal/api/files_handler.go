package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yourorg/panel-daemon/internal/files"
)

func (h *Handlers) ListFiles(w http.ResponseWriter, r *http.Request) {
	serverUUID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return
	}

	entries, err := files.List(h.Docker.ServerVolumePath(serverUUID), r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, entries)
}

func (h *Handlers) ReadFile(w http.ResponseWriter, r *http.Request) {
	serverUUID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return
	}

	f, err := files.Read(h.Docker.ServerVolumePath(serverUUID), r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()

	if info, statErr := f.Stat(); statErr == nil {
		w.Header().Set("X-File-Mtime", strconv.FormatInt(info.ModTime().Unix(), 10))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, f)
}

func (h *Handlers) WriteFile(w http.ResponseWriter, r *http.Request) {
	serverUUID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return
	}

	var expectedMtime int64
	if raw := r.Header.Get("X-Expected-Mtime"); raw != "" {
		expectedMtime, _ = strconv.ParseInt(raw, 10, 64)
	}

	err = files.Write(h.Docker.ServerVolumePath(serverUUID), r.URL.Query().Get("path"), r.Body, expectedMtime)
	if errors.Is(err, files.ErrConflict) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) DeleteFile(w http.ResponseWriter, r *http.Request) {
	serverUUID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return
	}

	if err := files.Delete(h.Docker.ServerVolumePath(serverUUID), r.URL.Query().Get("path")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) CreateDirectory(w http.ResponseWriter, r *http.Request) {
	serverUUID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return
	}

	if err := files.Mkdir(h.Docker.ServerVolumePath(serverUUID), r.URL.Query().Get("path")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type renameFileRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (h *Handlers) RenameFile(w http.ResponseWriter, r *http.Request) {
	serverUUID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return
	}

	var req renameFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := files.Rename(h.Docker.ServerVolumePath(serverUUID), req.From, req.To); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
