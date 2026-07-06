package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/crypto"
	"github.com/yourorg/panel/internal/mysqlhost"
)

type DatabaseHostHandler struct {
	DB            *pgxpool.Pool
	EncryptionKey string
}

type databaseHostSummary struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	AdminUsername string `json:"admin_username"`
}

func (h *DatabaseHostHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(),
		`SELECT id, name, host, port, admin_username FROM database_hosts ORDER BY name`)
	if err != nil {
		http.Error(w, "failed to list database hosts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	hosts := make([]databaseHostSummary, 0)
	for rows.Next() {
		var host databaseHostSummary
		if err := rows.Scan(&host.ID, &host.Name, &host.Host, &host.Port, &host.AdminUsername); err != nil {
			http.Error(w, "failed to read database hosts", http.StatusInternalServerError)
			return
		}
		hosts = append(hosts, host)
	}

	writeJSON(w, http.StatusOK, hosts)
}

type createDatabaseHostRequest struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`
}

func (h *DatabaseHostHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createDatabaseHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Host == "" || req.AdminUsername == "" || req.AdminPassword == "" {
		http.Error(w, "name, host, admin_username and admin_password are required", http.StatusBadRequest)
		return
	}
	if req.Port == 0 {
		req.Port = 3306
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := mysqlhost.Ping(ctx, mysqlhost.Host{
		Hostname: req.Host, Port: req.Port,
		AdminUsername: req.AdminUsername, AdminPassword: req.AdminPassword,
	}); err != nil {
		http.Error(w, mysqlhost.DescribeConnectError(err), http.StatusBadGateway)
		return
	}

	passwordEncrypted, err := crypto.Encrypt(h.EncryptionKey, req.AdminPassword)
	if err != nil {
		http.Error(w, "failed to encrypt admin password", http.StatusInternalServerError)
		return
	}

	var id int64
	if err := h.DB.QueryRow(r.Context(), `
		INSERT INTO database_hosts (name, host, port, admin_username, admin_password_encrypted)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		req.Name, req.Host, req.Port, req.AdminUsername, passwordEncrypted,
	).Scan(&id); err != nil {
		http.Error(w, "failed to create database host", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *DatabaseHostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid database host id", http.StatusBadRequest)
		return
	}

	var count int
	if err := h.DB.QueryRow(r.Context(),
		`SELECT count(*) FROM server_databases WHERE database_host_id = $1`, id,
	).Scan(&count); err != nil {
		http.Error(w, "failed to check host's databases", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "this host still has databases provisioned on it — delete them first", http.StatusConflict)
		return
	}

	res, err := h.DB.Exec(r.Context(), `DELETE FROM database_hosts WHERE id = $1`, id)
	if err != nil {
		http.Error(w, "failed to delete database host", http.StatusInternalServerError)
		return
	}
	if res.RowsAffected() == 0 {
		http.Error(w, "database host not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
