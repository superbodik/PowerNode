package domainretry

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/daemonclient"
)

type NodeClientResolver func(nodeID int64) (*daemonclient.Client, error)

const checkInterval = 15 * time.Minute

func Run(pool *pgxpool.Pool, resolveNodeClient NodeClientResolver) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for range ticker.C {
		retryPending(pool, resolveNodeClient)
	}
}

type pendingDomain struct {
	ID         int64
	Domain     string
	AdminEmail string
	ServerUUID uuid.UUID
	NodeID     int64
	Port       int
}

func retryPending(pool *pgxpool.Pool, resolveNodeClient NodeClientResolver) {
	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT sd.id, sd.domain, sd.admin_email, s.uuid, s.node_id,
		       (SELECT port FROM allocations a WHERE a.server_id = s.id ORDER BY a.id LIMIT 1)
		FROM server_domains sd
		JOIN servers s ON s.id = sd.server_id
		WHERE sd.tls_status != 'active' AND sd.created_at > now() - interval '2 hours'`,
	)
	if err != nil {
		log.Printf("domainretry: query failed: %v", err)
		return
	}

	var pending []pendingDomain
	for rows.Next() {
		var d pendingDomain
		var port *int
		if err := rows.Scan(&d.ID, &d.Domain, &d.AdminEmail, &d.ServerUUID, &d.NodeID, &port); err != nil {
			continue
		}
		if port == nil {
			continue
		}
		d.Port = *port
		pending = append(pending, d)
	}
	rows.Close()

	for _, d := range pending {
		retryOne(ctx, pool, resolveNodeClient, d)
	}
}

func retryOne(ctx context.Context, pool *pgxpool.Pool, resolveNodeClient NodeClientResolver, d pendingDomain) {
	client, err := resolveNodeClient(d.NodeID)
	if err != nil {
		log.Printf("domainretry: domain %s: node unavailable: %v", d.Domain, err)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	resp, err := client.AddDomain(reqCtx, d.ServerUUID, daemonclient.AddDomainRequest{
		Domain: d.Domain, Port: d.Port, Email: d.AdminEmail,
	})
	if err != nil {
		log.Printf("domainretry: domain %s: retry failed: %v", d.Domain, err)
		return
	}

	if _, err := pool.Exec(ctx,
		`UPDATE server_domains SET tls_status = $1 WHERE id = $2`, resp.TLSStatus, d.ID,
	); err != nil {
		log.Printf("domainretry: domain %s: failed to persist status: %v", d.Domain, err)
		return
	}
	if resp.TLSStatus == "active" {
		log.Printf("domainretry: domain %s: certificate issued on retry", d.Domain)
	}
}
