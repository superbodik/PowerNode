#!/usr/bin/env bash

DB_NAME="panel"
DB_USER="panel"

install_database() {
	log_step "Installing PostgreSQL and Redis"

	if ! require_command psql; then
		apt-get install -y -qq postgresql postgresql-contrib || die "PostgreSQL installation failed"
	fi
	systemctl enable --now postgresql 2>/dev/null
	wait_for_postgres
	log_ok "PostgreSQL installed and running"

	if ! require_command redis-cli; then
		apt-get install -y -qq redis-server || die "Redis installation failed"
	fi
	systemctl enable --now redis-server 2>/dev/null
	if ! timeout 10 bash -c 'until redis-cli ping >/dev/null 2>&1; do sleep 1; done'; then
		die "Redis did not start — check 'systemctl status redis-server' / 'journalctl -u redis-server'"
	fi
	log_ok "Redis installed and running"
}

wait_for_postgres() {
	local i
	for i in $(seq 1 15); do
		pg_isready -q 2>/dev/null && return 0
		sleep 1
	done

	log_warn "PostgreSQL not responding after 15s — checking for an existing cluster (pg_lsclusters)"
	if ! pg_lsclusters 2>/dev/null | grep -q .; then
		local pg_version
		pg_version=$(ls /usr/lib/postgresql/ 2>/dev/null | sort -V | tail -n1)
		[[ -n "$pg_version" ]] || die "No PostgreSQL binaries found under /usr/lib/postgresql/ — installation is broken, rerun 'apt-get install --reinstall postgresql'"

		log_warn "No cluster found — creating one (pg_createcluster ${pg_version} main --start)"
		pg_createcluster --locale C.UTF-8 "$pg_version" main --start \
			|| die "pg_createcluster failed (see the error above) — this is usually a locale or permissions problem on /var/lib/postgresql"
	else
		log_warn "A cluster exists but isn't responding — trying 'service postgresql restart'"
		service postgresql restart || die "'service postgresql restart' failed (see the error above)"
	fi

	for i in $(seq 1 15); do
		pg_isready -q 2>/dev/null && return 0
		sleep 1
	done

	die "PostgreSQL still isn't accepting connections after creating/restarting the cluster. Run 'journalctl -u postgresql -n 40 --no-pager' for the real error."
}

provision_database() {
	local db_password="$1"

	local role_exists
	role_exists=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'")
	if [[ "$role_exists" != "1" ]]; then
		sudo -u postgres psql -c "CREATE ROLE ${DB_USER} WITH LOGIN PASSWORD '${db_password}';" \
			|| die "Failed to create database role"
	else
		sudo -u postgres psql -c "ALTER ROLE ${DB_USER} WITH PASSWORD '${db_password}';"
	fi

	local db_exists
	db_exists=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'")
	if [[ "$db_exists" != "1" ]]; then
		sudo -u postgres psql -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};" \
			|| die "Failed to create database"
	fi
	log_ok "Database role and database ready (${DB_USER}@${DB_NAME})"

	local migration="${PROJECT_ROOT}/backend/migrations/0001_init.sql"
	if [[ -f "$migration" ]]; then
		PGPASSWORD="$db_password" psql -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -f "$migration" \
			|| die "Failed to apply migration $migration"
		log_ok "Applied migration: $(basename "$migration")"
	else
		log_warn "Migration file not found at $migration — skipping schema load"
	fi

	PGPASSWORD="$db_password" psql -h 127.0.0.1 -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 \
		-c "INSERT INTO locations (short_code, description) VALUES ('default', 'Default location') ON CONFLICT (short_code) DO NOTHING;" \
		|| die "Failed to seed default location"
	log_ok "Seeded default location ('default')"
}
