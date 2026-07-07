package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/crypto"
	"github.com/yourorg/panel/internal/mysqlhost"
)

type ServerDatabaseHandler struct {
	DB       *pgxpool.Pool
	Subusers *auth.SubuserChecker
	Encrypt  string
}

func (h *ServerDatabaseHandler) resolveServer(w http.ResponseWriter, r *http.Request, permission string) (int64, bool) {
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
	if !claims.HasKeyPermission(permission) || !h.Subusers.CanAccessServer(r.Context(), claims, ownerID, serverID, permission) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return 0, false
	}

	return serverID, true
}

type serverDatabaseSummary struct {
	ID           int64  `json:"id"`
	DatabaseName string `json:"database_name"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
}

func (h *ServerDatabaseHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.resolveServer(w, r, auth.PermDatabasesRead)
	if !ok {
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT sd.id, sd.database_name, sd.username, sd.password_encrypted, dh.host, dh.port
		FROM server_databases sd JOIN database_hosts dh ON dh.id = sd.database_host_id
		WHERE sd.server_id = $1 ORDER BY sd.created_at`, serverID)
	if err != nil {
		http.Error(w, "failed to list databases", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	databases := make([]serverDatabaseSummary, 0)
	for rows.Next() {
		var d serverDatabaseSummary
		var passwordEncrypted string
		if err := rows.Scan(&d.ID, &d.DatabaseName, &d.Username, &passwordEncrypted, &d.Host, &d.Port); err != nil {
			http.Error(w, "failed to read databases", http.StatusInternalServerError)
			return
		}
		password, err := crypto.Decrypt(h.Encrypt, passwordEncrypted)
		if err != nil {
			http.Error(w, "failed to decrypt database password", http.StatusInternalServerError)
			return
		}
		d.Password = password
		databases = append(databases, d)
	}

	writeJSON(w, http.StatusOK, databases)
}

var dbNameSuffixPattern = regexp.MustCompile(`[^a-zA-Z0-9_]`)

type createServerDatabaseRequest struct {
	DatabaseHostID int64  `json:"database_host_id"`
	Name           string `json:"name"`
}

func (h *ServerDatabaseHandler) Create(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.resolveServer(w, r, auth.PermDatabasesWrite)
	if !ok {
		return
	}

	var req createServerDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DatabaseHostID == 0 || req.Name == "" {
		http.Error(w, "database_host_id and name are required", http.StatusBadRequest)
		return
	}

	suffix := dbNameSuffixPattern.ReplaceAllString(req.Name, "")
	if len(suffix) > 16 {
		suffix = suffix[:16]
	}
	if suffix == "" {
		http.Error(w, "name must contain at least one letter, digit, or underscore", http.StatusBadRequest)
		return
	}

	databaseName := "s" + strconv.FormatInt(serverID, 10) + "_" + suffix
	username := "u" + strconv.FormatInt(serverID, 10) + "_" + suffix
	if len(username) > 32 {
		username = username[:32]
	}
	if len(databaseName) > 64 {
		databaseName = databaseName[:64]
	}

	rawPassword, err := generateToken(24)
	if err != nil {
		http.Error(w, "failed to generate password", http.StatusInternalServerError)
		return
	}

	var hostname string
	var port int
	var adminUsername, adminPasswordEncrypted string
	if err := h.DB.QueryRow(r.Context(),
		`SELECT host, port, admin_username, admin_password_encrypted FROM database_hosts WHERE id = $1`,
		req.DatabaseHostID,
	).Scan(&hostname, &port, &adminUsername, &adminPasswordEncrypted); err != nil {
		http.Error(w, "database host not found", http.StatusNotFound)
		return
	}
	adminPassword, err := crypto.Decrypt(h.Encrypt, adminPasswordEncrypted)
	if err != nil {
		http.Error(w, "failed to decrypt host admin password", http.StatusInternalServerError)
		return
	}

	passwordEncrypted, err := crypto.Encrypt(h.Encrypt, rawPassword)
	if err != nil {
		http.Error(w, "failed to encrypt database password", http.StatusInternalServerError)
		return
	}

	var id int64
	if err := h.DB.QueryRow(r.Context(), `
		INSERT INTO server_databases (server_id, database_host_id, database_name, username, password_encrypted)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		serverID, req.DatabaseHostID, databaseName, username, passwordEncrypted,
	).Scan(&id); err != nil {
		http.Error(w, "failed to save database record", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := mysqlhost.Provision(ctx, mysqlhost.Host{
		Hostname: hostname, Port: port,
		AdminUsername: adminUsername, AdminPassword: adminPassword,
	}, databaseName, username, rawPassword); err != nil {
		h.DB.Exec(r.Context(), `DELETE FROM server_databases WHERE id = $1`, id)
		http.Error(w, "failed to provision database: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusCreated, serverDatabaseSummary{
		ID: id, DatabaseName: databaseName, Username: username,
		Password: rawPassword, Host: hostname, Port: port,
	})
}

func (h *ServerDatabaseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	serverID, ok := h.resolveServer(w, r, auth.PermDatabasesWrite)
	if !ok {
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid database id", http.StatusBadRequest)
		return
	}

	var databaseName, username string
	var hostname string
	var port int
	var adminUsername, adminPasswordEncrypted string
	if err := h.DB.QueryRow(r.Context(), `
		SELECT sd.database_name, sd.username, dh.host, dh.port, dh.admin_username, dh.admin_password_encrypted
		FROM server_databases sd JOIN database_hosts dh ON dh.id = sd.database_host_id
		WHERE sd.id = $1 AND sd.server_id = $2`, id, serverID,
	).Scan(&databaseName, &username, &hostname, &port, &adminUsername, &adminPasswordEncrypted); err != nil {
		http.Error(w, "database not found", http.StatusNotFound)
		return
	}

	adminPassword, err := crypto.Decrypt(h.Encrypt, adminPasswordEncrypted)
	if err != nil {
		http.Error(w, "failed to decrypt host admin password", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := mysqlhost.Deprovision(ctx, mysqlhost.Host{
		Hostname: hostname, Port: port,
		AdminUsername: adminUsername, AdminPassword: adminPassword,
	}, databaseName, username); err != nil {
		http.Error(w, "failed to deprovision database: "+err.Error(), http.StatusBadGateway)
		return
	}

	if _, err := h.DB.Exec(r.Context(), `DELETE FROM server_databases WHERE id = $1`, id); err != nil {
		http.Error(w, "failed to delete database record", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
