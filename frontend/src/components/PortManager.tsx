import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ServerAllocation } from '../types';

interface Props {
  uuid: string;
}

export function PortManager({ uuid }: Props) {
  const [allocations, setAllocations] = useState<ServerAllocation[] | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [port, setPort] = useState('');
  const [alias, setAlias] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [deletingId, setDeletingId] = useState<number | null>(null);

  function refresh() {
    api
      .listServerAllocations(uuid)
      .then((a) => {
        setAllocations(a);
        setForbidden(false);
      })
      .catch(() => {
        setAllocations(null);
        setForbidden(true);
      });
  }

  useEffect(refresh, [uuid]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const result = await api.createServerAllocation(uuid, Number(port), alias.trim() || undefined);
      setAllocations(result);
      setPort('');
      setAlias('');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(a: ServerAllocation) {
    const warning = a.is_primary
      ? `Remove port ${a.port}? This is the server's main address — its listed address will change. The container will be recreated to apply this and will briefly restart.`
      : `Remove port ${a.port}? The server's container will be recreated to apply this — it will briefly restart.`;
    if (!window.confirm(warning)) {
      return;
    }
    setDeletingId(a.id);
    setError(null);
    try {
      await api.deleteServerAllocation(uuid, a.id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setDeletingId(null);
    }
  }

  if (forbidden) {
    return (
      <p className="srv-desc">
        You don't have permission to view this server's ports.
      </p>
    );
  }

  if (allocations === null) {
    return <p className="srv-desc">Loading…</p>;
  }

  return (
    <div>
      {error && <div className="login-error show" style={{ marginBottom: 12 }}>{error}</div>}

      <div className="settings-card" style={{ marginBottom: 20 }}>
        <div className="settings-card-title">Add a port</div>
        <p className="srv-desc" style={{ marginBottom: 12 }}>
          Publishes a TCP/UDP port from your server's container to the node's network.
          Applying this recreates the container (files are kept) and briefly restarts it.
        </p>
        <form onSubmit={handleCreate}>
          <div className="settings-grid">
            <div className="sfield">
              <label htmlFor="alloc-port">Port</label>
              <input
                id="alloc-port"
                type="number"
                min={1024}
                max={65535}
                value={port}
                onChange={(e) => setPort(e.target.value)}
                placeholder="25566"
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="alloc-alias">Label (optional)</label>
              <input
                id="alloc-alias"
                value={alias}
                onChange={(e) => setAlias(e.target.value)}
                placeholder="query port"
              />
            </div>
          </div>
          <div className="settings-foot">
            <button
              className="btn-primary"
              type="submit"
              disabled={submitting}
              style={{ width: 'auto', padding: '10px 20px' }}
            >
              {submitting ? 'Adding…' : 'Add port'}
            </button>
          </div>
        </form>
      </div>

      <div className="sch-list">
        {allocations.map((a) => (
          <div className="sch-card" key={a.id}>
            <div className="sch-head">
              <span className="sch-name">
                {a.ip}:{a.port}
                {a.is_primary && (
                  <span className="srv-desc" style={{ fontSize: 10, marginLeft: 8, border: '1px solid var(--border)', borderRadius: 4, padding: '1px 5px' }}>
                    Main
                  </span>
                )}
              </span>
              <button
                className="file-act-btn del"
                onClick={() => handleDelete(a)}
                disabled={deletingId === a.id}
              >
                {deletingId === a.id ? 'Removing…' : 'Delete'}
              </button>
            </div>
            {a.alias && (
              <div className="sch-meta">
                <span>{a.alias}</span>
              </div>
            )}
          </div>
        ))}
        {allocations.length === 0 && <p className="srv-desc">No ports assigned yet.</p>}
      </div>
    </div>
  );
}
