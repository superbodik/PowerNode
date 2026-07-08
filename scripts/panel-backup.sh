#!/usr/bin/env bash
set -uo pipefail

PANEL_ENV_FILE="/etc/panel/panel.env"
BACKUP_DIR="/var/backups/panel"
KEEP_DAYS=14

if [[ ! -f "$PANEL_ENV_FILE" ]]; then
	echo "panel-backup: $PANEL_ENV_FILE not found — is the panel installed on this host?" >&2
	exit 1
fi

db_url=$(grep '^PANEL_DATABASE_URL=' "$PANEL_ENV_FILE" | cut -d= -f2-)
if [[ -z "$db_url" ]]; then
	echo "panel-backup: PANEL_DATABASE_URL not set in $PANEL_ENV_FILE" >&2
	exit 1
fi

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"

timestamp=$(date -u +%Y%m%dT%H%M%SZ)
dump_file="${BACKUP_DIR}/db-${timestamp}.sql.gz"
env_file="${BACKUP_DIR}/env-${timestamp}.tar.gz"

if ! pg_dump "$db_url" | gzip > "$dump_file"; then
	echo "panel-backup: pg_dump failed" >&2
	rm -f "$dump_file"
	exit 1
fi
chmod 600 "$dump_file"

if ! tar -czf "$env_file" -C /etc panel; then
	echo "panel-backup: failed to archive /etc/panel" >&2
	rm -f "$env_file"
	exit 1
fi
chmod 600 "$env_file"

echo "panel-backup: wrote $dump_file and $env_file"

find "$BACKUP_DIR" -name '*.sql.gz' -mtime +"$KEEP_DAYS" -delete
find "$BACKUP_DIR" -name '*.tar.gz' -mtime +"$KEEP_DAYS" -delete
