import type {
  ActivityEntry,
  Allocation,
  ApiKey,
  CreateAllocationRequest,
  CreateApiKeyResponse,
  CreateNodeRequest,
  CreateNodeResponse,
  CreateScheduleRequest,
  CreateServerRequest,
  Egg,
  FileEntry,
  Node,
  NodeStatus,
  PowerAction,
  Schedule,
  Server,
  TwoFASetup,
  TwoFAStatus,
  UpdateCheck,
  VersionInfo,
} from '../types';

const API_BASE = '/api/v1';

interface AuthTokens {
  access_token: string;
  refresh_token: string;
  user: { id: number; email: string; username: string };
}

export class TOTPRequiredError extends Error {
  constructor() {
    super('totp code required');
    this.name = 'TOTPRequiredError';
  }
}

function authHeaders(): HeadersInit {
  const token = localStorage.getItem('access_token');
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function storeTokens(tokens: AuthTokens) {
  localStorage.setItem('access_token', tokens.access_token);
  localStorage.setItem('refresh_token', tokens.refresh_token);
  localStorage.setItem('user', JSON.stringify(tokens.user));
}

function clearTokens() {
  localStorage.removeItem('access_token');
  localStorage.removeItem('refresh_token');
  localStorage.removeItem('user');
}

let refreshInFlight: Promise<boolean> | null = null;

async function tryRefresh(): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight;

  refreshInFlight = (async () => {
    const refreshToken = localStorage.getItem('refresh_token');
    if (!refreshToken) return false;
    try {
      const res = await fetch(`${API_BASE}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
      if (!res.ok) return false;
      storeTokens((await res.json()) as AuthTokens);
      return true;
    } catch {
      return false;
    }
  })();

  const result = await refreshInFlight;
  refreshInFlight = null;
  return result;
}

async function request<T>(path: string, init?: RequestInit, isRetry = false): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders(),
      ...init?.headers,
    },
  });

  if (res.status === 401) {
    if (!isRetry && (await tryRefresh())) {
      return request<T>(path, init, true);
    }
    clearTokens();
    window.location.reload();
    throw new Error('session expired');
  }
  if (!res.ok) {
    throw new Error(`${init?.method ?? 'GET'} ${path} failed: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

async function requestText(path: string, init?: RequestInit, isRetry = false): Promise<string> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      ...authHeaders(),
      ...init?.headers,
    },
  });

  if (res.status === 401) {
    if (!isRetry && (await tryRefresh())) {
      return requestText(path, init, true);
    }
    clearTokens();
    window.location.reload();
    throw new Error('session expired');
  }
  if (!res.ok) {
    throw new Error(`${init?.method ?? 'GET'} ${path} failed: ${res.status}`);
  }
  return res.text();
}

async function requestBlob(path: string, init?: RequestInit, isRetry = false): Promise<Blob> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      ...authHeaders(),
      ...init?.headers,
    },
  });

  if (res.status === 401) {
    if (!isRetry && (await tryRefresh())) {
      return requestBlob(path, init, true);
    }
    clearTokens();
    window.location.reload();
    throw new Error('session expired');
  }
  if (!res.ok) {
    throw new Error(`${init?.method ?? 'GET'} ${path} failed: ${res.status}`);
  }
  return res.blob();
}

export const api = {
  login: async (email: string, password: string, totpCode?: string) => {
    const res = await fetch(`${API_BASE}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password, totp_code: totpCode ?? '' }),
    });
    if (res.status === 428) {
      throw new TOTPRequiredError();
    }
    if (!res.ok) {
      throw new Error('invalid credentials');
    }
    return (await res.json()) as AuthTokens;
  },

  listServers: () => request<Server[]>('/servers'),

  getServer: (uuid: string) => request<Server>(`/servers/${uuid}`),

  createServer: (payload: CreateServerRequest) =>
    request<{ id: number; uuid: string }>('/servers', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  power: (uuid: string, action: PowerAction) =>
    request<{ success: boolean; state: string }>(`/servers/${uuid}/power`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    }),

  deleteServer: (uuid: string) => request<void>(`/servers/${uuid}`, { method: 'DELETE' }),

  listNodes: () => request<Node[]>('/nodes'),

  createNode: (payload: CreateNodeRequest) =>
    request<CreateNodeResponse>('/nodes', { method: 'POST', body: JSON.stringify(payload) }),

  checkNodeStatus: (id: number) => request<NodeStatus>(`/nodes/${id}/status`),

  getVersion: () => request<VersionInfo>('/version'),

  checkUpdate: () => request<UpdateCheck>('/version/check'),

  listActivity: () => request<ActivityEntry[]>('/activity'),

  listEggs: () => request<Egg[]>('/eggs'),

  listAllocations: (nodeId: number, freeOnly = false) =>
    request<Allocation[]>(`/allocations?node_id=${nodeId}${freeOnly ? '&free=true' : ''}`),

  createAllocation: (payload: CreateAllocationRequest) =>
    request<{ id: number }>('/allocations', { method: 'POST', body: JSON.stringify(payload) }),

  listApiKeys: () => request<ApiKey[]>('/account/api-keys'),

  createApiKey: (name: string) =>
    request<CreateApiKeyResponse>('/account/api-keys', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),

  deleteApiKey: (id: number) => request<void>(`/account/api-keys/${id}`, { method: 'DELETE' }),

  get2FAStatus: () => request<TwoFAStatus>('/account/2fa/status'),

  setup2FA: () => request<TwoFASetup>('/account/2fa/setup', { method: 'POST' }),

  verify2FA: (code: string) =>
    request<{ enabled: boolean }>('/account/2fa/verify', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  disable2FA: (password: string) =>
    request<{ enabled: boolean }>('/account/2fa/disable', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),

  listFiles: (uuid: string, path: string) =>
    request<FileEntry[]>(`/servers/${uuid}/files?path=${encodeURIComponent(path)}`),

  readFile: (uuid: string, path: string) =>
    requestText(`/servers/${uuid}/files/contents?path=${encodeURIComponent(path)}`),

  writeFile: (uuid: string, path: string, content: string) =>
    requestText(`/servers/${uuid}/files/contents?path=${encodeURIComponent(path)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: content,
    }),

  deleteFile: (uuid: string, path: string) =>
    request<void>(`/servers/${uuid}/files?path=${encodeURIComponent(path)}`, { method: 'DELETE' }),

  createDirectory: (uuid: string, path: string) =>
    request<void>(`/servers/${uuid}/files/directory?path=${encodeURIComponent(path)}`, {
      method: 'POST',
    }),

  renameFile: (uuid: string, from: string, to: string) =>
    request<void>(`/servers/${uuid}/files/rename`, {
      method: 'POST',
      body: JSON.stringify({ from, to }),
    }),

  downloadFile: (uuid: string, path: string) =>
    requestBlob(`/servers/${uuid}/files/contents?path=${encodeURIComponent(path)}`),

  uploadFile: (uuid: string, path: string, file: File) =>
    requestText(`/servers/${uuid}/files/contents?path=${encodeURIComponent(path)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: file,
    }),

  listSchedules: (uuid: string) => request<Schedule[]>(`/servers/${uuid}/schedules`),

  createSchedule: (uuid: string, payload: CreateScheduleRequest) =>
    request<{ id: number }>(`/servers/${uuid}/schedules`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  toggleSchedule: (uuid: string, id: number) =>
    request<void>(`/servers/${uuid}/schedules/${id}/toggle`, { method: 'POST' }),

  deleteSchedule: (uuid: string, id: number) =>
    request<void>(`/servers/${uuid}/schedules/${id}`, { method: 'DELETE' }),
};

export { storeTokens, clearTokens };

function wsToken(): string {
  return localStorage.getItem('access_token') ?? '';
}

export function connectServerSocket(uuid: string): WebSocket {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return new WebSocket(`${proto}://${window.location.host}/ws/servers/${uuid}?token=${wsToken()}`);
}

export function connectConsoleSocket(uuid: string): WebSocket {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return new WebSocket(
    `${proto}://${window.location.host}/ws/servers/${uuid}/console?token=${wsToken()}`,
  );
}
