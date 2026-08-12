import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { GuidePanel } from './GuidePanel';
import { t } from '../i18n';
import type { DatabaseHost, ServerDatabase } from '../types';

interface Props {
  uuid: string;
}

export function DatabaseManager({ uuid }: Props) {
  const [databases, setDatabases] = useState<ServerDatabase[] | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [hosts, setHosts] = useState<DatabaseHost[]>([]);
  const [hostId, setHostId] = useState(0);
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [revealed, setRevealed] = useState<Record<number, boolean>>({});

  function refresh() {
    api
      .listServerDatabases(uuid)
      .then((d) => {
        setDatabases(d);
        setForbidden(false);
      })
      .catch(() => {
        setDatabases(null);
        setForbidden(true);
      });
  }

  useEffect(refresh, [uuid]);
  useEffect(() => {
    api.listDatabaseHosts().then(setHosts).catch(() => setHosts([]));
  }, []);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.createServerDatabase(uuid, hostId, name);
      setName('');
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(db: ServerDatabase) {
    if (!window.confirm(t('db.confirmDelete', { name: db.database_name }))) {
      return;
    }
    try {
      await api.deleteServerDatabase(uuid, db.id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  if (forbidden) {
    return (
      <p className="srv-desc">
        {t('db.forbidden')}
      </p>
    );
  }

  if (databases === null) {
    return <p className="srv-desc">{t('common.loading')}</p>;
  }

  return (
    <div>
      <GuidePanel title={t('guide.databases.title')}>
        <p>{t('guide.databases.p1')}</p>
        <p>{t('guide.databases.p2')}</p>
        <p>{t('guide.databases.p3')}</p>
        <p>{t('guide.databases.p4')}</p>
      </GuidePanel>
      {error && <div className="login-error show" style={{ marginBottom: 12 }}>{error}</div>}

      {hosts.length === 0 ? (
        <p className="srv-desc">
          {t('db.noHostsYet')}
        </p>
      ) : (
        <div className="settings-card" style={{ marginBottom: 20 }}>
          <div className="settings-card-title">{t('db.createDatabase')}</div>
          <form onSubmit={handleCreate}>
            <div className="settings-grid">
              <div className="sfield">
                <label htmlFor="db-host">{t('db.host')}</label>
                <select id="db-host" value={hostId} onChange={(e) => setHostId(Number(e.target.value))} required>
                  <option value={0} disabled>
                    {t('db.selectHost')}
                  </option>
                  {hosts.map((h) => (
                    <option key={h.id} value={h.id}>
                      {h.name}
                    </option>
                  ))}
                </select>
              </div>
              <div className="sfield">
                <label htmlFor="db-name">{t('db.name')}</label>
                <input
                  id="db-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t('db.namePlaceholder')}
                  required
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
                {submitting ? t('db.creatingDatabase') : t('db.createDatabase')}
              </button>
            </div>
          </form>
        </div>
      )}

      <div className="sch-list">
        {databases.map((db) => (
          <div className="sch-card" key={db.id}>
            <div className="sch-head">
              <span className="sch-name">{db.database_name}</span>
              <button className="file-act-btn del" onClick={() => handleDelete(db)}>
                {t('common.delete')}
              </button>
            </div>
            <div className="sch-meta" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
              <span>{t('db.hostLine', { host: db.host, port: db.port })}</span>
              <span>{t('db.userLine', { username: db.username })}</span>
              <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                {t('db.passwordLine')}{revealed[db.id] ? db.password : '••••••••••••'}
                <button
                  className="btn-sm"
                  type="button"
                  onClick={() => setRevealed((r) => ({ ...r, [db.id]: !r[db.id] }))}
                >
                  {revealed[db.id] ? t('common.hide') : t('common.show')}
                </button>
                <button
                  className="btn-sm"
                  type="button"
                  onClick={() => navigator.clipboard?.writeText(db.password)}
                >
                  {t('common.copy')}
                </button>
              </span>
            </div>
          </div>
        ))}
        {databases.length === 0 && <p className="srv-desc">{t('db.noDatabasesYet')}</p>}
      </div>
    </div>
  );
}
