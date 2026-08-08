package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/activity"
	"github.com/yourorg/panel/internal/auth"
	"github.com/yourorg/panel/internal/daemonclient"
)

// ServerAllocationHandler lets a server's owner manage the ports published for
// their own server, instead of only getting the single allocation picked at
// creation time.
type ServerAllocationHandler struct {
	DB         *pgxpool.Pool
	Subusers   *auth.SubuserChecker
	NodeClient func(nodeID int64) (*daemonclient.Client, error)
}

const (
	minUserPort = 1024
	maxUserPort = 65535
)

// Ports the node itself needs; handing one to a container would either fail to
// bind or take down the daemon's own listeners.
var reservedNodePorts = map[int]string{
	22:   "SSH",
	80:   "the node's HTTP proxy",
	443:  "the node's HTTPS proxy",
	2022: "SFTP",
}

type serverAllocationSummary struct {
	ID        int64   `json:"id"`
	IP        string  `json:"ip"`
	Port      int     `json:"port"`
	Alias     *string `json:"alias"`
	IsPrimary bool    `json:"is_primary"`
	IsOwn     bool    `json:"is_own"`
}

func (h *ServerAllocationHandler) resolveServer(w http.ResponseWriter, r *http.Request, permission string) (serverID, nodeID int64, serverUUID uuid.UUID, ok bool) {
	claims, authOK := auth.FromContext(r.Context())
	if !authOK {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, 0, uuid.UUID{}, false
	}

	parsedUUID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		http.Error(w, "invalid server uuid", http.StatusBadRequest)
		return 0, 0, uuid.UUID{}, false
	}

	var ownerID int64
	if err := h.DB.QueryRow(r.Context(),
		`SELECT id, owner_id, node_id FROM servers WHERE uuid = $1`, parsedUUID,
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

func (h *ServerAllocationHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID, _, _, ok := h.resolveServer(w, r, auth.PermAllocationsRead)
	if !ok {
		return
	}

	allocations, err := h.listAllocations(r.Context(), serverID)
	if err != nil {
		http.Error(w, "failed to list ports", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, allocations)
}

func (h *ServerAllocationHandler) listAllocations(ctx context.Context, serverID int64) ([]serverAllocationSummary, error) {
	rows, err := h.DB.Query(ctx,
		`SELECT id, HOST(ip), port, alias, is_user_created
		 FROM allocations WHERE server_id = $1 ORDER BY id`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allocations := make([]serverAllocationSummary, 0)
	for rows.Next() {
		var a serverAllocationSummary
		if err := rows.Scan(&a.ID, &a.IP, &a.Port, &a.Alias, &a.IsOwn); err != nil {
			return nil, err
		}
		allocations = append(allocations, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The panel treats the oldest allocation as the server's main address —
	// it's the one shown on the card and the one domains proxy to.
	if len(allocations) > 0 {
		allocations[0].IsPrimary = true
	}
	return allocations, nil
}

type createServerAllocationRequest struct {
	Port  int    `json:"port"`
	Alias string `json:"alias"`
}

func (h *ServerAllocationHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	serverID, nodeID, serverUUID, ok := h.resolveServer(w, r, auth.PermAllocationsWrite)
	if !ok {
		return
	}

	var req createServerAllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Port < minUserPort || req.Port > maxUserPort {
		http.Error(w, fmt.Sprintf("port must be between %d and %d", minUserPort, maxUserPort), http.StatusBadRequest)
		return
	}
	if used, reserved := reservedNodePorts[req.Port]; reserved {
		http.Error(w, "port "+strconv.Itoa(req.Port)+" is reserved for "+used, http.StatusBadRequest)
		return
	}
	alias := strings.TrimSpace(req.Alias)
	if len(alias) > 64 {
		http.Error(w, "alias must be 64 characters or fewer", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var isSuspended bool
	var allocationLimit int
	if err := h.DB.QueryRow(ctx,
		`SELECT is_suspended, allocation_limit FROM servers WHERE id = $1`, serverID,
	).Scan(&isSuspended, &allocationLimit); err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	if isSuspended {
		http.Error(w, "this server is suspended and its ports cannot be changed", http.StatusConflict)
		return
	}

	var nodeDaemonPort int
	var nodeFQDN string
	if err := h.DB.QueryRow(ctx,
		`SELECT daemon_port, fqdn FROM nodes WHERE id = $1`, nodeID,
	).Scan(&nodeDaemonPort, &nodeFQDN); err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	if req.Port == nodeDaemonPort {
		http.Error(w, "port "+strconv.Itoa(req.Port)+" is reserved for the node daemon", http.StatusBadRequest)
		return
	}

	ip, err := h.allocationIP(ctx, serverID, nodeID, nodeFQDN)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	var used int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM allocations WHERE server_id = $1`, serverID,
	).Scan(&used); err != nil {
		http.Error(w, "failed to count ports", http.StatusInternalServerError)
		return
	}
	if !claims.IsAdmin && used >= allocationLimit {
		http.Error(w, fmt.Sprintf("this server is limited to %d port(s)", allocationLimit), http.StatusConflict)
		return
	}

	var aliasValue *string
	if alias != "" {
		aliasValue = &alias
	}

	// Claims a free allocation from the node's pool if the admin already added
	// this port, otherwise records a new one owned by this server.
	var allocationID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO allocations (node_id, ip, port, alias, server_id, is_user_created)
		VALUES ($1, $2, $3, $4, $5, TRUE)
		ON CONFLICT (node_id, ip, port) DO UPDATE
			SET server_id = EXCLUDED.server_id,
			    alias = COALESCE(EXCLUDED.alias, allocations.alias)
			WHERE allocations.server_id IS NULL
		RETURNING id`,
		nodeID, ip, req.Port, aliasValue, serverID,
	).Scan(&allocationID)
	if err == pgx.ErrNoRows {
		http.Error(w, "port "+strconv.Itoa(req.Port)+" is already in use on this node", http.StatusConflict)
		return
	} else if err != nil {
		http.Error(w, "failed to add port", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "failed to add port", http.StatusInternalServerError)
		return
	}

	if err := h.applyPorts(ctx, serverID, nodeID, serverUUID); err != nil {
		h.releaseAllocation(ctx, allocationID)
		http.Error(w, "failed to publish the new port on the node: "+err.Error(), http.StatusBadGateway)
		return
	}

	activity.Record(ctx, h.DB, activity.Entry{
		ActorUserID: &claims.UserID,
		ServerID:    &serverID,
		NodeID:      &nodeID,
		Event:       "server.allocation.create",
		IPAddress:   activity.RequestIP(r),
		Metadata:    map[string]any{"port": req.Port},
	})

	allocations, err := h.listAllocations(ctx, serverID)
	if err != nil {
		http.Error(w, "port added but failed to reload the list", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, allocations)
}

func (h *ServerAllocationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	serverID, nodeID, serverUUID, ok := h.resolveServer(w, r, auth.PermAllocationsWrite)
	if !ok {
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid port id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	var isSuspended bool
	if err := h.DB.QueryRow(ctx,
		`SELECT is_suspended FROM servers WHERE id = $1`, serverID,
	).Scan(&isSuspended); err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	if isSuspended {
		http.Error(w, "this server is suspended and its ports cannot be changed", http.StatusConflict)
		return
	}

	allocations, err := h.listAllocations(ctx, serverID)
	if err != nil {
		http.Error(w, "failed to load ports", http.StatusInternalServerError)
		return
	}

	var target *serverAllocationSummary
	for i := range allocations {
		if allocations[i].ID == id {
			target = &allocations[i]
			break
		}
	}
	if target == nil {
		http.Error(w, "port not found on this server", http.StatusNotFound)
		return
	}

	if target.IsPrimary {
		var domainCount int
		if err := h.DB.QueryRow(ctx,
			`SELECT count(*) FROM server_domains WHERE server_id = $1`, serverID,
		).Scan(&domainCount); err != nil {
			http.Error(w, "failed to check domains", http.StatusInternalServerError)
			return
		}
		if domainCount > 0 {
			http.Error(w, "this is the server's main port and domains are pointed at it — remove those domains first", http.StatusConflict)
			return
		}
	}

	// A port the owner added is theirs to drop; one from the node's pool goes
	// back to the pool for the next server.
	if target.IsOwn {
		_, err = h.DB.Exec(ctx, `DELETE FROM allocations WHERE id = $1 AND server_id = $2`, id, serverID)
	} else {
		_, err = h.DB.Exec(ctx, `UPDATE allocations SET server_id = NULL WHERE id = $1 AND server_id = $2`, id, serverID)
	}
	if err != nil {
		http.Error(w, "failed to remove port", http.StatusInternalServerError)
		return
	}

	if err := h.applyPorts(ctx, serverID, nodeID, serverUUID); err != nil {
		http.Error(w, "port removed from the panel but the node could not be updated: "+err.Error(), http.StatusBadGateway)
		return
	}

	activity.Record(ctx, h.DB, activity.Entry{
		ActorUserID: &claims.UserID,
		ServerID:    &serverID,
		NodeID:      &nodeID,
		Event:       "server.allocation.delete",
		IPAddress:   activity.RequestIP(r),
		Metadata:    map[string]any{"port": target.Port},
	})

	w.WriteHeader(http.StatusNoContent)
}

// allocationIP picks the address new ports are published on: the one the server
// already uses, else the address this node's existing allocations use, else the
// node's own address when it is a bare IP.
func (h *ServerAllocationHandler) allocationIP(ctx context.Context, serverID, nodeID int64, nodeFQDN string) (string, error) {
	var ip string
	err := h.DB.QueryRow(ctx,
		`SELECT HOST(ip) FROM allocations WHERE server_id = $1 ORDER BY id LIMIT 1`, serverID,
	).Scan(&ip)
	if err == nil {
		return ip, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("failed to resolve an address for this port")
	}

	err = h.DB.QueryRow(ctx,
		`SELECT HOST(ip) FROM allocations WHERE node_id = $1
		 GROUP BY ip ORDER BY count(*) DESC, ip LIMIT 1`, nodeID,
	).Scan(&ip)
	if err == nil {
		return ip, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("failed to resolve an address for this port")
	}

	if net.ParseIP(nodeFQDN) != nil {
		return nodeFQDN, nil
	}
	return "", fmt.Errorf("this node has no allocation address configured yet — ask an admin to add one on the Nodes page")
}

// applyPorts republishes the server's container with its current set of ports.
func (h *ServerAllocationHandler) applyPorts(ctx context.Context, serverID, nodeID int64, serverUUID uuid.UUID) error {
	var dockerImage, startupCommand string
	var environmentJSON []byte
	var memoryMB, swapMB int64
	var ioWeight int
	var cpuPercent *int
	if err := h.DB.QueryRow(ctx, `
		SELECT docker_image, startup_command, environment, memory_mb, swap_mb, io_weight, cpu_percent
		FROM servers WHERE id = $1`, serverID,
	).Scan(&dockerImage, &startupCommand, &environmentJSON, &memoryMB, &swapMB, &ioWeight, &cpuPercent); err != nil {
		return fmt.Errorf("load server: %w", err)
	}

	environment := map[string]string{}
	if err := json.Unmarshal(environmentJSON, &environment); err != nil {
		return fmt.Errorf("read server environment: %w", err)
	}

	rows, err := h.DB.Query(ctx, `SELECT port FROM allocations WHERE server_id = $1 ORDER BY id`, serverID)
	if err != nil {
		return fmt.Errorf("load ports: %w", err)
	}
	defer rows.Close()

	portBindings := map[string]string{}
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return fmt.Errorf("read ports: %w", err)
		}
		portBindings[fmt.Sprintf("%d/tcp", port)] = strconv.Itoa(port)
		portBindings[fmt.Sprintf("%d/udp", port)] = strconv.Itoa(port)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read ports: %w", err)
	}

	client, err := h.NodeClient(nodeID)
	if err != nil {
		return fmt.Errorf("node unavailable: %w", err)
	}

	resp, err := client.RebuildServer(ctx, serverUUID, daemonclient.CreateServerRequest{
		ServerUUID:     serverUUID,
		DockerImage:    dockerImage,
		StartupCommand: startupCommand,
		Environment:    environment,
		MemoryMB:       memoryMB,
		SwapMB:         swapMB,
		IOWeight:       ioWeight,
		CPUPercent:     derefInt(cpuPercent),
		PortBindings:   portBindings,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Message)
	}
	return nil
}

// releaseAllocation undoes a just-created allocation when the node refused it,
// so a failed add doesn't leave a port booked against the server.
func (h *ServerAllocationHandler) releaseAllocation(ctx context.Context, allocationID int64) {
	res, err := h.DB.Exec(ctx, `DELETE FROM allocations WHERE id = $1 AND is_user_created`, allocationID)
	if err != nil {
		log.Printf("allocations: failed to roll back allocation %d: %v", allocationID, err)
		return
	}
	if res.RowsAffected() > 0 {
		return
	}
	if _, err := h.DB.Exec(ctx,
		`UPDATE allocations SET server_id = NULL WHERE id = $1`, allocationID); err != nil {
		log.Printf("allocations: failed to release allocation %d: %v", allocationID, err)
	}
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
