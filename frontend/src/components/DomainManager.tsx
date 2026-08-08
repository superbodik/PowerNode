import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ServerAllocation, ServerDomain } from '../types';

interface Props {
  uuid: string;
}

export function DomainManager({ uuid }: Props) {
  const [domains, setDomains] = useState<ServerDomain[] | null>(null);
  const [allocations, setAllocations] = useState<ServerAllocation[]>([]);
  const [forbidden, setForbidden] = useState(false);
  const [domain, setDomain] = useState('');
  const [email, setEmail] = useState('');
  const [allocationId, setAllocationId] = useState<number | ''>('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function refresh() {
    api
      .listServerDomains(uuid)
      .then((d) => {
        setDomains(d);
        setForbidden(false);
      })
      .catch(() => {
        setDomains(null);
        setForbidden(true);
      });
  }

  useEffect(refresh, [uuid]);

  useEffect(() => {
    api
      .listServerAllocations(uuid)
      .then((list) => {
        setAllocations(list);
        const primary = list.find((a) => a.is_primary);
        if (primary) setAllocationId(primary.id);
      })
      .catch(() => setAllocations([]));
  }, [uuid]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.createServerDomain(uuid, domain, email, allocationId === '' ? undefined : allocationId);
      setDomain('');
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(d: ServerDomain) {
    if (!window.confirm(`Remove "${d.domain}"? This deletes its reverse proxy and TLS certificate.`)) {
      return;
    }
    try {
      await api.deleteServerDomain(uuid, d.id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  if (forbidden) {
    return (
      <p className="srv-desc">
        You don't have permission to view this server's domains.
      </p>
    );
  }

  if (domains === null) {
    return <p className="srv-desc">Loading…</p>;
  }

  return (
    <div>
      {error && <div className="login-error show" style={{ marginBottom: 12 }}>{error}</div>}

      <div className="settings-card" style={{ marginBottom: 20 }}>
        <div className="settings-card-title">Add a custom domain</div>
        <p className="srv-desc" style={{ marginBottom: 12 }}>
          Point the domain's DNS A record at this server's node before adding it here, or
          certificate issuance will fail and it'll stay on plain HTTP. Subdomains work too —
          add each one separately and point it at a different port below to run several
          services off the same server.
        </p>
        <form onSubmit={handleCreate}>
          <div className="settings-grid">
            <div className="sfield">
              <label htmlFor="domain-name">Domain</label>
              <input
                id="domain-name"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                placeholder="sub.example.com"
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="domain-port">Port</label>
              <select
                id="domain-port"
                value={allocationId}
                onChange={(e) => setAllocationId(e.target.value === '' ? '' : Number(e.target.value))}
                required
              >
                <option value="" disabled>
                  Select a port…
                </option>
                {allocations.map((a) => (
                  <option key={a.id} value={a.id}>
                    :{a.port}
                    {a.is_primary ? ' (main)' : ''}
                    {a.alias ? ` — ${a.alias}` : ''}
                  </option>
                ))}
              </select>
            </div>
            <div className="sfield">
              <label htmlFor="domain-email">Contact email (optional)</label>
              <input
                id="domain-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="for Let's Encrypt renewal notices"
              />
            </div>
          </div>
          {allocations.length === 0 && (
            <p className="srv-desc" style={{ marginTop: 8 }}>
              This server has no ports yet — add one on the Network tab first.
            </p>
          )}
          <div className="settings-foot">
            <button
              className="btn-primary"
              type="submit"
              disabled={submitting || allocations.length === 0}
              style={{ width: 'auto', padding: '10px 20px' }}
            >
              {submitting ? 'Adding…' : 'Add domain'}
            </button>
          </div>
        </form>
      </div>

      <div className="sch-list">
        {domains.map((d) => (
          <div className="sch-card" key={d.id}>
            <div className="sch-head">
              <span className="sch-name">
                {d.domain}
                <span className="srv-desc" style={{ fontSize: 10, marginLeft: 8, border: '1px solid var(--border)', borderRadius: 4, padding: '1px 5px' }}>
                  :{d.port}
                </span>
              </span>
              <button className="file-act-btn del" onClick={() => handleDelete(d)}>
                Delete
              </button>
            </div>
            <div className="sch-meta">
              <span>TLS: {d.tls_status === 'active' ? 'HTTPS active' : 'HTTP only'}</span>
              <span>Added: {new Date(d.created_at).toLocaleDateString()}</span>
            </div>
          </div>
        ))}
        {domains.length === 0 && <p className="srv-desc">No custom domains yet.</p>}
      </div>
    </div>
  );
}
