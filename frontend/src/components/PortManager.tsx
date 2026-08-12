import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { GuidePanel } from './GuidePanel';
import { t } from '../i18n';
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
      ? t('ports.confirmRemovePrimary', { port: a.port })
      : t('ports.confirmRemove', { port: a.port });
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
        {t('ports.forbidden')}
      </p>
    );
  }

  if (allocations === null) {
    return <p className="srv-desc">{t('common.loading')}</p>;
  }

  return (
    <div>
      <GuidePanel title={t('guide.network.title')}>
        <p>{t('guide.network.p1')}</p>
        <p>{t('guide.network.p2')}</p>
        <p>{t('guide.network.p3')}</p>
      </GuidePanel>
      {error && <div className="login-error show" style={{ marginBottom: 12 }}>{error}</div>}

      <div className="settings-card" style={{ marginBottom: 20 }}>
        <div className="settings-card-title">{t('ports.addPort')}</div>
        <p className="srv-desc" style={{ marginBottom: 12 }}>
          {t('ports.hint')}
        </p>
        <form onSubmit={handleCreate}>
          <div className="settings-grid">
            <div className="sfield">
              <label htmlFor="alloc-port">{t('ports.port')}</label>
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
              <label htmlFor="alloc-alias">{t('ports.labelOptional')}</label>
              <input
                id="alloc-alias"
                value={alias}
                onChange={(e) => setAlias(e.target.value)}
                placeholder={t('ports.labelPlaceholder')}
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
              {submitting ? t('ports.adding') : t('ports.addPortButton')}
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
                    {t('ports.main')}
                  </span>
                )}
              </span>
              <button
                className="file-act-btn del"
                onClick={() => handleDelete(a)}
                disabled={deletingId === a.id}
              >
                {deletingId === a.id ? t('ports.removing') : t('common.delete')}
              </button>
            </div>
            {a.alias && (
              <div className="sch-meta">
                <span>{a.alias}</span>
              </div>
            )}
          </div>
        ))}
        {allocations.length === 0 && <p className="srv-desc">{t('ports.noPortsAssignedYet')}</p>}
      </div>
    </div>
  );
}
