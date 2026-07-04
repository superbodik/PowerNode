CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    email           CITEXT NOT NULL UNIQUE,
    username        CITEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    is_admin        BOOLEAN NOT NULL DEFAULT FALSE,
    totp_secret     TEXT,
    totp_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at   TIMESTAMPTZ,
    last_login_ip   INET,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE roles (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    description     TEXT
);

CREATE TABLE permissions (
    id              SERIAL PRIMARY KEY,
    code            TEXT NOT NULL UNIQUE,
    description     TEXT
);

CREATE TABLE role_permissions (
    role_id         INT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id   INT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_roles (
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id         INT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE server_subusers (
    id              BIGSERIAL PRIMARY KEY,
    server_id       BIGINT NOT NULL,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permissions     JSONB NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, user_id)
);

CREATE TABLE ssh_keys (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    public_key      TEXT NOT NULL,
    fingerprint     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    token_hash      TEXT NOT NULL UNIQUE,
    permissions     JSONB NOT NULL DEFAULT '[]',
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE locations (
    id              SERIAL PRIMARY KEY,
    short_code      TEXT NOT NULL UNIQUE,
    description     TEXT
);

CREATE TABLE nodes (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    name                TEXT NOT NULL,
    location_id         INT NOT NULL REFERENCES locations(id),
    fqdn                TEXT NOT NULL,
    scheme              TEXT NOT NULL DEFAULT 'https' CHECK (scheme IN ('http','https')),
    daemon_port         INT NOT NULL DEFAULT 8443,
    daemon_token_hash   TEXT NOT NULL,
    memory_mb           BIGINT NOT NULL,
    memory_overallocate INT NOT NULL DEFAULT 0,
    disk_mb              BIGINT NOT NULL,
    disk_overallocate   INT NOT NULL DEFAULT 0,
    cpu_percent         INT,
    is_public           BOOLEAN NOT NULL DEFAULT TRUE,
    maintenance_mode    BOOLEAN NOT NULL DEFAULT FALSE,
    last_seen_at        TIMESTAMPTZ,
    agent_version       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE allocations (
    id              BIGSERIAL PRIMARY KEY,
    node_id         BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    ip              INET NOT NULL,
    port            INT NOT NULL,
    alias           TEXT,
    server_id       BIGINT,
    UNIQUE (node_id, ip, port)
);

CREATE TABLE eggs (
    id                  SERIAL PRIMARY KEY,
    uuid                UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    category            TEXT NOT NULL,
    name                TEXT NOT NULL,
    description         TEXT,
    docker_image        TEXT NOT NULL,
    docker_images       JSONB NOT NULL DEFAULT '{}',
    startup_command     TEXT NOT NULL,
    stop_command        TEXT NOT NULL DEFAULT 'stop',
    config_files        JSONB NOT NULL DEFAULT '{}',
    config_startup      JSONB NOT NULL DEFAULT '{}',
    install_script       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE egg_variables (
    id              BIGSERIAL PRIMARY KEY,
    egg_id          INT NOT NULL REFERENCES eggs(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    env_variable    TEXT NOT NULL,
    default_value   TEXT NOT NULL DEFAULT '',
    is_editable     BOOLEAN NOT NULL DEFAULT TRUE,
    rules           TEXT NOT NULL DEFAULT '',
    UNIQUE (egg_id, env_variable)
);

CREATE TYPE server_status AS ENUM (
    'installing', 'install_failed', 'suspended',
    'offline', 'starting', 'running', 'stopping'
);

CREATE TABLE servers (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    uuid_short          TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    description         TEXT,
    owner_id            BIGINT NOT NULL REFERENCES users(id),
    node_id             BIGINT NOT NULL REFERENCES nodes(id),
    egg_id              INT NOT NULL REFERENCES eggs(id),
    docker_image        TEXT NOT NULL,
    startup_command     TEXT NOT NULL,
    environment         JSONB NOT NULL DEFAULT '{}',

    memory_mb           BIGINT NOT NULL,
    swap_mb              BIGINT NOT NULL DEFAULT 0,
    disk_mb              BIGINT NOT NULL,
    io_weight            INT NOT NULL DEFAULT 500,
    cpu_percent         INT,
    threads_pinned      TEXT,

    allocation_limit    INT NOT NULL DEFAULT 1,
    database_limit      INT NOT NULL DEFAULT 0,
    backup_limit        INT NOT NULL DEFAULT 1,

    status              server_status NOT NULL DEFAULT 'installing',
    container_id        TEXT,
    is_suspended        BOOLEAN NOT NULL DEFAULT FALSE,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE server_subusers
    ADD CONSTRAINT fk_server_subusers_server
    FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;

ALTER TABLE allocations
    ADD CONSTRAINT fk_allocations_server
    FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE SET NULL;

CREATE TABLE server_variables (
    server_id           BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    egg_variable_id     BIGINT NOT NULL REFERENCES egg_variables(id) ON DELETE CASCADE,
    value               TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (server_id, egg_variable_id)
);

CREATE TABLE server_databases (
    id                  BIGSERIAL PRIMARY KEY,
    server_id           BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    database_host_id    BIGINT,
    database_name       TEXT NOT NULL,
    username            TEXT NOT NULL,
    password_encrypted  TEXT NOT NULL,
    remote_pattern      TEXT NOT NULL DEFAULT '%',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE server_backups (
    id                  BIGSERIAL PRIMARY KEY,
    uuid                UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    server_id           BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    ignored_files       JSONB NOT NULL DEFAULT '[]',
    bytes               BIGINT NOT NULL DEFAULT 0,
    checksum            TEXT,
    is_successful       BOOLEAN NOT NULL DEFAULT FALSE,
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE server_schedules (
    id                  BIGSERIAL PRIMARY KEY,
    server_id           BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    cron_minute         TEXT NOT NULL DEFAULT '*',
    cron_hour           TEXT NOT NULL DEFAULT '*',
    cron_day_of_week    TEXT NOT NULL DEFAULT '*',
    cron_day_of_month   TEXT NOT NULL DEFAULT '*',
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    only_when_online    BOOLEAN NOT NULL DEFAULT FALSE,
    last_run_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE schedule_tasks (
    id                  BIGSERIAL PRIMARY KEY,
    schedule_id         BIGINT NOT NULL REFERENCES server_schedules(id) ON DELETE CASCADE,
    sequence_id         INT NOT NULL,
    action              TEXT NOT NULL,
    payload             TEXT NOT NULL DEFAULT '',
    time_offset_seconds INT NOT NULL DEFAULT 0
);

CREATE TABLE activity_logs (
    id                  BIGSERIAL PRIMARY KEY,
    actor_user_id       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    server_id           BIGINT REFERENCES servers(id) ON DELETE SET NULL,
    node_id             BIGINT REFERENCES nodes(id) ON DELETE SET NULL,
    event               TEXT NOT NULL,
    ip_address          INET,
    metadata            JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_servers_owner       ON servers(owner_id);
CREATE INDEX idx_servers_node        ON servers(node_id);
CREATE INDEX idx_servers_status      ON servers(status);
CREATE INDEX idx_allocations_node    ON allocations(node_id) WHERE server_id IS NULL;
CREATE INDEX idx_activity_created    ON activity_logs(created_at DESC);
CREATE INDEX idx_activity_server     ON activity_logs(server_id, created_at DESC);
