export type ServerStatus =
  | 'installing'
  | 'install_failed'
  | 'suspended'
  | 'offline'
  | 'starting'
  | 'running'
  | 'stopping';

export interface Server {
  id: number;
  uuid: string;
  uuid_short: string;
  name: string;
  description?: string;
  owner_id: number;
  node_id: number;
  egg_id: number;
  docker_image: string;
  startup_command: string;
  environment: Record<string, string>;

  memory_mb: number;
  swap_mb: number;
  disk_mb: number;
  io_weight: number;
  cpu_percent?: number | null;
  threads_pinned?: string;

  allocation_limit: number;
  database_limit: number;
  backup_limit: number;

  status: ServerStatus;
  container_id?: string;
  is_suspended: boolean;

  created_at: string;
  updated_at: string;

  live?: ResourceStats;
  node_name?: string;
  primary_address?: string;
}

export interface ResourceStats {
  server_uuid: string;
  cpu_percent: number;
  memory_bytes: number;
  disk_bytes: number;
  network_rx: number;
  network_tx: number;
  uptime_seconds: number;
  state: ServerStatus;
}

export type PowerAction = 'start' | 'stop' | 'restart' | 'kill';

export interface Node {
  id: number;
  name: string;
  fqdn: string;
  scheme: string;
  daemon_port: number;
  memory_mb: number;
  disk_mb: number;
  maintenance_mode: boolean;
  last_seen_at: string | null;
}

export interface CreateNodeRequest {
  name: string;
  location_id: number;
  fqdn: string;
  scheme?: string;
  daemon_port?: number;
  memory_mb: number;
  disk_mb: number;
}

export interface CreateNodeResponse {
  id: number;
  daemon_token: string;
}
