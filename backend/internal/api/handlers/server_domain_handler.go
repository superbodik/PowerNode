package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/daemonclient"
)

type ServerDomainHandler struct {
	DB         *pgxpool.Pool
	Subusers   *auth.SubuserChecker
	NodeClient func(nodeID int64) (*daemonclient.Client, error)
}

var domainPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

func validDomain(domain string) bool {
	return len(domain) <= 253 && domainPattern.MatchString(domain)
}

func (h *ServerDomainHandler) resolveServer(w http.ResponseWriter, r *http.Request, permission string) (serverID, nodeID int64, serverUUID uuid.UUID, ok bool) {
	claims, authOK := auth.FromContext(r.Context())
	if !authOK {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, 0, uuid.UUID{}, false
	}

	rawUUID := chi.URLParam(r, "uuid")
	parsedUUID, err := uuid.Parse(rawUUID)
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return 0, 0, uuid.UUID{}, false
	}

	var ownerID int64
	if err := h.DB.QueryRow(r.Context(),
		`SELECT id, owner_id, node_id FROM servers WHERE uuid = $1`, rawUUID,
	).Scan(&serverID, &ownerID, &nodeID); err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return 0, 0, uuid.UUID{}, false
	}
	if !claims.HasKeyPermission(permission) || !h.Subusers.CanAccessServer(r.Context(), claims, ownerID, serverID, permission) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return 0, 0, uuid.UUID{}, false
	}

	return serverID, nodeID, parsedUUID, true
}

type serverDomainSummary struct {
	ID        int64  `json:"id"`
	Domain    string `json:"domain"`
	TLSStatus string `json:"tls_status"`
	CreatedAt string `json:"created_at"`
}

func (h *ServerDomainHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID, _, _, ok := h.resolveServer(w, r, auth.PermDomainsRead)
	if !ok {
		return
	}

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, domain, tls_status, created_at FROM server_domains WHERE server_id = $1 ORDER BY created_at`, serverID)
	if err != nil {
		http.Error(w, "failed to list domains", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	domains := make([]serverDomainSummary, 0)
	for rows.Next() {
		var d serverDomainSummary
		var createdAt time.Time
		if err := rows.Scan(&d.ID, &d.Domain, &d.TLSStatus, &createdAt); err != nil {
			http.Error(w, "failed to read domains", http.StatusInternalServerError)
			return
		}
		d.CreatedAt = createdAt.Format(time.RFC3339)
		domains = append(domains, d)
	}

	writeJSON(w, http.StatusOK, domains)
}

type createServerDomainRequest struct {
	Domain string `json:"domain"`
	Email  string `json:"email"`
}

func (h *ServerDomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	serverID, nodeID, serverUUID, ok := h.resolveServer(w, r, auth.PermDomainsWrite)
	if !ok {
		return
	}

	var req createServerDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if !validDomain(domain) {
		http.Error(w, "not a valid domain name", http.StatusBadRequest)
		return
	}

	var port int
	if err := h.DB.QueryRow(r.Context(),
		`SELECT port FROM allocations WHERE server_id = $1 ORDER BY id LIMIT 1`, serverID,
	).Scan(&port); err != nil {
		http.Error(w, "this server has no allocation to attach a domain to", http.StatusConflict)
		return
	}

	client, err := h.NodeClient(nodeID)
	if err != nil {
		http.Error(w, "node unavailable: "+err.Error(), http.StatusBadGateway)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	resp, err := client.AddDomain(ctx, serverUUID, daemonclient.AddDomainRequest{Domain: domain, Port: port, Email: req.Email})
	if err != nil {
		http.Error(w, "daemon call failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	var id int64
	var createdAt time.Time
	if err := h.DB.QueryRow(r.Context(), `
		INSERT INTO server_domains (server_id, domain, tls_status)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`,
		serverID, domain, resp.TLSStatus,
	).Scan(&id, &createdAt); err != nil {
		http.Error(w, "domain already in use", http.StatusConflict)
		return
	}

	writeJSON(w, http.StatusCreated, serverDomainSummary{
		ID: id, Domain: domain, TLSStatus: resp.TLSStatus, CreatedAt: createdAt.Format(time.RFC3339),
	})
}

func (h *ServerDomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	serverID, nodeID, serverUUID, ok := h.resolveServer(w, r, auth.PermDomainsWrite)
	if !ok {
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid domain id", http.StatusBadRequest)
		return
	}

	var domain string
	if err := h.DB.QueryRow(r.Context(),
		`SELECT domain FROM server_domains WHERE id = $1 AND server_id = $2`, id, serverID,
	).Scan(&domain); err != nil {
		http.Error(w, "domain not found", http.StatusNotFound)
		return
	}

	client, err := h.NodeClient(nodeID)
	if err != nil {
		http.Error(w, "node unavailable: "+err.Error(), http.StatusBadGateway)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.RemoveDomain(ctx, serverUUID, domain); err != nil {
		http.Error(w, "daemon call failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	if _, err := h.DB.Exec(r.Context(), `DELETE FROM server_domains WHERE id = $1`, id); err != nil {
		http.Error(w, "failed to delete domain record", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
