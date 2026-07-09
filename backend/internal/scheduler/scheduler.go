package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/panel/internal/daemonclient"
	"github.com/yourorg/panel/internal/mail"
)

type NodeClientResolver func(nodeID int64) (*daemonclient.Client, error)

func Run(pool *pgxpool.Pool, resolveNodeClient NodeClientResolver, mailer *mail.Mailer) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	lastNodeUp := make(map[int64]bool)
	for range ticker.C {
		checkSchedules(pool, resolveNodeClient, mailer)
		checkNodeHealth(pool, resolveNodeClient, mailer, lastNodeUp)
	}
}

func checkNodeHealth(pool *pgxpool.Pool, resolveNodeClient NodeClientResolver, mailer *mail.Mailer, lastUp map[int64]bool) {
	if mailer == nil || !mailer.Enabled() {
		return
	}

	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT id, name FROM nodes`)
	if err != nil {
		log.Printf("scheduler: node health query failed: %v", err)
		return
	}
	type node struct {
		ID   int64
		Name string
	}
	var nodes []node
	for rows.Next() {
		var n node
		if err := rows.Scan(&n.ID, &n.Name); err == nil {
			nodes = append(nodes, n)
		}
	}
	rows.Close()

	for _, n := range nodes {
		up := pingNode(ctx, resolveNodeClient, n.ID)
		wasUp, known := lastUp[n.ID]
		lastUp[n.ID] = up
		if known && wasUp && !up {
			notifyAdmins(pool, mailer, fmt.Sprintf("Node %q unreachable", n.Name),
				fmt.Sprintf("Node %q (id %d) stopped responding to health checks.", n.Name, n.ID))
		}
	}
}

func pingNode(ctx context.Context, resolveNodeClient NodeClientResolver, nodeID int64) bool {
	client, err := resolveNodeClient(nodeID)
	if err != nil {
		return false
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err = client.Ping(pingCtx)
	return err == nil
}

func notifyAdmins(pool *pgxpool.Pool, mailer *mail.Mailer, subject, body string) {
	rows, err := pool.Query(context.Background(), `SELECT email FROM users WHERE is_admin = true AND is_active = true`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			continue
		}
		if err := mailer.Send(email, subject, body); err != nil {
			log.Printf("scheduler: failed to send node-health email to %s: %v", email, err)
		}
	}
}

func notifyOwner(pool *pgxpool.Pool, mailer *mail.Mailer, serverID int64, serverName, subject, body string) {
	if mailer == nil || !mailer.Enabled() {
		return
	}
	var email string
	if err := pool.QueryRow(context.Background(), `
		SELECT u.email FROM servers s JOIN users u ON u.id = s.owner_id WHERE s.id = $1`, serverID,
	).Scan(&email); err != nil {
		return
	}
	if err := mailer.Send(email, fmt.Sprintf("[%s] %s", serverName, subject), body); err != nil {
		log.Printf("scheduler: failed to send notification email to %s: %v", email, err)
	}
}

type dueSchedule struct {
	ID             int64
	ServerID       int64
	OnlyWhenOnline bool
}

func cronFieldMatches(field string, value int) bool {
	if field == "" || field == "*" {
		return true
	}
	n, err := strconv.Atoi(field)
	return err == nil && n == value
}

func checkSchedules(pool *pgxpool.Pool, resolveNodeClient NodeClientResolver, mailer *mail.Mailer) {
	ctx := context.Background()
	now := time.Now().UTC()
	minuteStart := now.Truncate(time.Minute)

	rows, err := pool.Query(ctx, `
		SELECT id, server_id, cron_minute, cron_hour, cron_day_of_week, cron_day_of_month, only_when_online
		FROM server_schedules
		WHERE is_active = true AND (last_run_at IS NULL OR last_run_at < $1)`, minuteStart)
	if err != nil {
		log.Printf("scheduler: query failed: %v", err)
		return
	}

	var due []dueSchedule
	for rows.Next() {
		var (
			id, serverID                           int64
			cronMinute, cronHour, cronDOW, cronDOM string
			onlyWhenOnline                         bool
		)
		if err := rows.Scan(&id, &serverID, &cronMinute, &cronHour, &cronDOW, &cronDOM, &onlyWhenOnline); err != nil {
			continue
		}
		if cronFieldMatches(cronMinute, now.Minute()) &&
			cronFieldMatches(cronHour, now.Hour()) &&
			cronFieldMatches(cronDOW, int(now.Weekday())) &&
			cronFieldMatches(cronDOM, now.Day()) {
			due = append(due, dueSchedule{ID: id, ServerID: serverID, OnlyWhenOnline: onlyWhenOnline})
		}
	}
	rows.Close()

	for _, s := range due {
		tag, err := pool.Exec(ctx,
			`UPDATE server_schedules SET last_run_at = $1
			 WHERE id = $2 AND (last_run_at IS NULL OR last_run_at < $1)`, minuteStart, s.ID)
		if err != nil || tag.RowsAffected() == 0 {
			continue
		}
		go execute(pool, resolveNodeClient, mailer, s.ID, s.ServerID, s.OnlyWhenOnline)
	}
}

type scheduledTask struct {
	Action  string
	Payload string
	Offset  int
}

func execute(pool *pgxpool.Pool, resolveNodeClient NodeClientResolver, mailer *mail.Mailer, scheduleID, serverID int64, onlyWhenOnline bool) {
	ctx := context.Background()

	var nodeID int64
	var serverUUID uuid.UUID
	var status, serverName string
	if err := pool.QueryRow(ctx, `SELECT node_id, uuid, status, name FROM servers WHERE id = $1`, serverID).
		Scan(&nodeID, &serverUUID, &status, &serverName); err != nil {
		log.Printf("scheduler: schedule %d: server lookup failed: %v", scheduleID, err)
		return
	}
	if onlyWhenOnline && status != "running" {
		return
	}

	client, err := resolveNodeClient(nodeID)
	if err != nil {
		log.Printf("scheduler: schedule %d: node unavailable: %v", scheduleID, err)
		notifyOwner(pool, mailer, serverID, serverName, "Scheduled task failed", fmt.Sprintf("Could not reach the node to run a scheduled task: %v", err))
		return
	}

	rows, err := pool.Query(ctx,
		`SELECT action, payload, time_offset_seconds FROM schedule_tasks
		 WHERE schedule_id = $1 ORDER BY sequence_id`, scheduleID)
	if err != nil {
		log.Printf("scheduler: schedule %d: task lookup failed: %v", scheduleID, err)
		return
	}
	var tasks []scheduledTask
	for rows.Next() {
		var t scheduledTask
		if err := rows.Scan(&t.Action, &t.Payload, &t.Offset); err == nil {
			tasks = append(tasks, t)
		}
	}
	rows.Close()

	for _, t := range tasks {
		if t.Offset > 0 {
			time.Sleep(time.Duration(t.Offset) * time.Second)
		}
		switch t.Action {
		case "power":
			if _, err := client.Power(ctx, serverUUID, daemonclient.PowerAction(t.Payload)); err != nil {
				log.Printf("scheduler: schedule %d: power %q failed: %v", scheduleID, t.Payload, err)
				notifyOwner(pool, mailer, serverID, serverName, "Scheduled power action failed",
					fmt.Sprintf("The scheduled %q action failed: %v", t.Payload, err))
			}
		case "command":
			if err := client.SendCommand(ctx, serverUUID, t.Payload); err != nil {
				log.Printf("scheduler: schedule %d: command %q failed: %v", scheduleID, t.Payload, err)
			}
		case "backup":
			name := t.Payload
			if name == "" {
				name = "scheduled-" + time.Now().UTC().Format("2006-01-02T15-04-05Z")
			}
			if err := runScheduledBackup(ctx, pool, client, serverID, serverUUID, name); err != nil {
				log.Printf("scheduler: schedule %d: backup failed: %v", scheduleID, err)
				notifyOwner(pool, mailer, serverID, serverName, "Scheduled backup failed",
					fmt.Sprintf("The scheduled backup %q failed: %v", name, err))
			}
		}
	}
}

func runScheduledBackup(ctx context.Context, pool *pgxpool.Pool, client *daemonclient.Client, serverID int64, serverUUID uuid.UUID, name string) error {
	var backupLimit, count int
	if err := pool.QueryRow(ctx, `SELECT backup_limit FROM servers WHERE id = $1`, serverID).Scan(&backupLimit); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM server_backups WHERE server_id = $1`, serverID).Scan(&count); err != nil {
		return err
	}
	if count >= backupLimit {
		return fmt.Errorf("backup limit (%d) reached", backupLimit)
	}

	backupUUID := uuid.New()
	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO server_backups (uuid, server_id, name) VALUES ($1, $2, $3) RETURNING id`,
		backupUUID, serverID, name,
	).Scan(&id); err != nil {
		return err
	}

	resp, err := client.CreateBackup(ctx, serverUUID, daemonclient.CreateBackupRequest{BackupUUID: backupUUID.String()})
	if err != nil {
		return fmt.Errorf("backup %d recorded but daemon call failed: %w", id, err)
	}

	_, err = pool.Exec(ctx,
		`UPDATE server_backups SET bytes = $1, checksum = $2, is_successful = true, completed_at = now() WHERE id = $3`,
		resp.Bytes, resp.Checksum, id)
	return err
}
