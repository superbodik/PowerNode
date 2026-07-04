import { useEffect, useMemo, useState } from 'react';
import { api, connectServerSocket } from '../api/client';
import type { PowerAction, ResourceStats, Server } from '../types';
import { ServerCard } from './ServerCard';

interface Props {
  onManage: (uuid: string) => void;
}

export function ServerList({ onManage }: Props) {
  const [servers, setServers] = useState<Server[]>([]);
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    api
      .listServers()
      .then((data) => {
        if (!cancelled) setServers(data);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const sockets = servers.map((server) => {
      const ws = connectServerSocket(server.uuid);
      ws.onmessage = (event) => {
        try {
          const stats: ResourceStats = JSON.parse(event.data);
          setServers((prev) =>
            prev.map((s) => (s.uuid === stats.server_uuid ? { ...s, live: stats, status: stats.state } : s)),
          );
        } catch {}
      };
      return ws;
    });
    return () => sockets.forEach((ws) => ws.close());
  }, [servers.map((s) => s.uuid).join(',')]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return servers;
    return servers.filter((s) => s.name.toLowerCase().includes(q) || s.uuid_short.includes(q));
  }, [servers, query]);

  const stats = useMemo(
    () => ({
      total: servers.length,
      online: servers.filter((s) => s.status === 'running').length,
      offline: servers.filter((s) => s.status === 'offline' || s.status === 'suspended').length,
    }),
    [servers],
  );

  async function handlePower(uuid: string, action: PowerAction) {
    setServers((prev) =>
      prev.map((s) => (s.uuid === uuid ? { ...s, status: action === 'stop' ? 'stopping' : 'starting' } : s)),
    );
    try {
      await api.power(uuid, action);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  if (loading) return <p className="srv-desc">Loading servers…</p>;
  if (error) return <div className="login-error show">{error}</div>;

  return (
    <div>
      <div className="dash-stats">
        <div className="stat-card">
          <div className="stat-card-val">{stats.total}</div>
          <div className="stat-card-lbl">Servers</div>
        </div>
        <div className="stat-card">
          <div className="stat-card-val">{stats.online}</div>
          <div className="stat-card-lbl">Online</div>
        </div>
        <div className="stat-card">
          <div className="stat-card-val">{stats.offline}</div>
          <div className="stat-card-lbl">Offline</div>
        </div>
      </div>

      <div className="dash-toolbar">
        <div className="search-wrap">
          <span className="search-icon">⌕</span>
          <input
            type="text"
            placeholder="Search servers…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
      </div>

      <div className="servers-grid">
        {filtered.map((server) => (
          <ServerCard key={server.uuid} server={server} onManage={onManage} onPower={handlePower} />
        ))}
        {filtered.length === 0 && <p className="srv-desc">No servers match your search.</p>}
      </div>
    </div>
  );
}
