import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ServerDomain } from '../types';

interface Props {
  uuid: string;
}

export function DomainManager({ uuid }: Props) {
  const [domains, setDomains] = useState<ServerDomain[] | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [domain, setDomain] = useState('');
  const [email, setEmail] = useState('');
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

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.createServerDomain(uuid, domain, email);
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
          certificate issuance will fail and it'll stay on plain HTTP.
        </p>
        <form onSubmit={handleCreate}>
          <div className="settings-grid">
            <div className="sfield">
              <label htmlFor="domain-name">Domain</label>
              <input
                id="domain-name"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                placeholder="example.com"
                required
              />
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
          <div className="settings-foot">
            <button
              className="btn-primary"
              type="submit"
              disabled={submitting}
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
              <span className="sch-name">{d.domain}</span>
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
