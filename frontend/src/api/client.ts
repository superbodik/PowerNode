import type {
  CreateNodeRequest,
  CreateNodeResponse,
  Node,
  PowerAction,
  Server,
  UpdateCheck,
  VersionInfo,
} from '../types';

const API_BASE = '/api/v1';

function authHeaders(): HeadersInit {
  const token = localStorage.getItem('access_token');
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders(),
      ...init?.headers,
    },
  });
  if (res.status === 401) {
    localStorage.removeItem('access_token');
    localStorage.removeItem('user');
    window.location.reload();
    throw new Error('session expired');
  }
  if (!res.ok) {
    throw new Error(`${init?.method ?? 'GET'} ${path} failed: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  login: (email: string, password: string) =>
    request<{ access_token: string; user: { id: number; email: string; username: string } }>(
      '/auth/login',
      { method: 'POST', body: JSON.stringify({ email, password }) },
    ),

  listServers: () => request<Server[]>('/servers'),

  getServer: (uuid: string) => request<Server>(`/servers/${uuid}`),

  power: (uuid: string, action: PowerAction) =>
    request<{ success: boolean; state: string }>(`/servers/${uuid}/power`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    }),

  listNodes: () => request<Node[]>('/nodes'),

  createNode: (payload: CreateNodeRequest) =>
    request<CreateNodeResponse>('/nodes', { method: 'POST', body: JSON.stringify(payload) }),

  getVersion: () => request<VersionInfo>('/version'),

  checkUpdate: () => request<UpdateCheck>('/version/check'),
};

export function connectServerSocket(uuid: string): WebSocket {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return new WebSocket(`${proto}://${window.location.host}/ws/servers/${uuid}`);
}
