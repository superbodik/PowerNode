import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { GuidePanel } from './GuidePanel';
import { t } from '../i18n';
import type { ServerBackup } from '../types';

interface Props {
  uuid: string;
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 MB';
  const mb = bytes / (1024 * 1024);
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb.toFixed(1)} MB`;
}

export function BackupManager({ uuid }: Props) {
  const [backups, setBackups] = useState<ServerBackup[] | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [name, setName] = useState('');
  const [ignoredFiles, setIgnoredFiles] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [busyId, setBusyId] = useState<number | null>(null);

  function refresh() {
    api
      .listServerBackups(uuid)
      .then((b) => {
        setBackups(b);
        setForbidden(false);
      })
      .catch(() => {
        setBackups(null);
        setForbidden(true);
      });
  }

  useEffect(refresh, [uuid]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreating(true);
    setError(null);
    try {
      const patterns = ignoredFiles
        .split(',')
        .map((p) => p.trim())
        .filter(Boolean);
      await api.createServerBackup(uuid, name, patterns);
      setName('');
      setIgnoredFiles('');
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setCreating(false);
    }
  }

  async function handleRestore(b: ServerBackup) {
    if (!window.confirm(t('backups.confirmRestore', { name: b.name }))) {
      return;
    }
    setBusyId(b.id);
    setError(null);
    try {
      await api.restoreServerBackup(uuid, b.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusyId(null);
    }
  }

  async function handleDelete(b: ServerBackup) {
    if (!window.confirm(t('backups.confirmDelete', { name: b.name }))) return;
    setBusyId(b.id);
    setError(null);
    try {
      await api.deleteServerBackup(uuid, b.id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusyId(null);
    }
  }

  async function handleDownload(b: ServerBackup) {
    setError(null);
    try {
      const blob = await api.downloadServerBackup(uuid, b.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${b.name}.tar.gz`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  if (forbidden) {
    return <p className="srv-desc">{t('backups.forbidden')}</p>;
  }

  if (backups === null) {
    return <p className="srv-desc">{t('common.loading')}</p>;
  }

  return (
    <div>
      <GuidePanel title={t('guide.backups.title')}>
        <p>{t('guide.backups.p1')}</p>
        <p>{t('guide.backups.p2')}</p>
        <p>{t('guide.backups.p3')}</p>
        <p>{t('guide.backups.p4')}</p>
      </GuidePanel>
      {error && <div className="login-error show" style={{ marginBottom: 12 }}>{error}</div>}

      <div className="settings-card" style={{ marginBottom: 20 }}>
        <div className="settings-card-title">{t('backups.createBackup')}</div>
        <p className="srv-desc" style={{ marginBottom: 12 }}>
          {t('backups.hint')}
        </p>
        <form onSubmit={handleCreate}>
          <div className="settings-grid">
            <div className="sfield">
              <label htmlFor="backup-name">{t('backups.name')}</label>
              <input
                id="backup-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('backups.namePlaceholder')}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="backup-ignored">{t('backups.ignore')}</label>
              <input
                id="backup-ignored"
                value={ignoredFiles}
                onChange={(e) => setIgnoredFiles(e.target.value)}
                placeholder={t('backups.ignorePlaceholder')}
              />
            </div>
          </div>
          <div className="settings-foot">
            <button
              className="btn-primary"
              type="submit"
              disabled={creating}
              style={{ width: 'auto', padding: '10px 20px' }}
            >
              {creating ? t('backups.creatingBackup') : t('backups.createBackup')}
            </button>
          </div>
        </form>
      </div>

      <div className="sch-list">
        {backups.map((b) => (
          <div className="sch-card" key={b.id}>
            <div className="sch-head">
              <span className="sch-name">{b.name}</span>
              <div style={{ display: 'flex', gap: 8 }}>
                <button
                  className="btn-sm"
                  disabled={!b.is_successful || busyId === b.id}
                  onClick={() => handleDownload(b)}
                >
                  {t('backups.download')}
                </button>
                <button
                  className="btn-sm"
                  disabled={!b.is_successful || busyId === b.id}
                  onClick={() => handleRestore(b)}
                >
                  {busyId === b.id ? t('backups.working') : t('backups.restore')}
                </button>
                <button className="file-act-btn del" disabled={busyId === b.id} onClick={() => handleDelete(b)}>
                  {t('common.delete')}
                </button>
              </div>
            </div>
            <div className="sch-meta">
              <span>{b.is_successful ? formatBytes(b.bytes) : t('backups.failed')}</span>
              <span>{t('backups.createdLine')}{new Date(b.created_at).toLocaleString()}</span>
            </div>
          </div>
        ))}
        {backups.length === 0 && <p className="srv-desc">{t('backups.noBackupsYet')}</p>}
      </div>
    </div>
  );
}
