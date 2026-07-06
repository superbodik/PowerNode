CREATE TABLE server_domains (
    id          BIGSERIAL PRIMARY KEY,
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    domain      TEXT NOT NULL UNIQUE,
    tls_status  TEXT NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_server_domains_server_id ON server_domains(server_id);
