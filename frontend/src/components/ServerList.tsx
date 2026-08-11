import { useEffect, useMemo, useState } from 'react';
import { api, connectServerSocketWithRetry } from '../api/client';
import { t } from '../i18n';
import type { PowerAction, ResourceStats, Server } from '../types';
import { ServerCard } from './ServerCard';

const POWER_LABEL_KEYS: Record<PowerAction, Parameters<typeof t>[0]> = {
  start: 'serverView.start',
  stop: 'serverView.stop',
  restart: 'serverView.restart',
  kill: 'serverView.kill',
};

interface Props {
  onManage: (uuid: string) => void;
}

export function ServerList({ onManage }: Props) {
  const [servers, setServers] = useState<Server[]>([]);
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkBusy, setBulkBusy] = useState(false);
  const [bulkError, setBulkError] = useState<string | null>(null);
  const [pendingActions, setPendingActions] = useState<Record<string, PowerAction>>({});

  useEffect(() => {
    api.me().then((me) => setIsAdmin(me.is_admin)).catch(() => {});
  }, []);

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
    const closers = servers.map((server) =>
      connectServerSocketWithRetry<ResourceStats>(server.uuid, (stats) => {
        setServers((prev) =>
          prev.map((s) => (s.uuid === stats.server_uuid ? { ...s, live: stats, status: stats.state } : s)),
        );
      }),
    );
    return () => closers.forEach((close) => close());
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
    if (pendingActions[uuid]) return;
    setPendingActions((prev) => ({ ...prev, [uuid]: action }));
    setServers((prev) =>
      prev.map((s) => (s.uuid === uuid ? { ...s, status: action === 'stop' ? 'stopping' : 'starting' } : s)),
    );
    try {
      await api.power(uuid, action);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPendingActions((prev) => {
        const next = { ...prev };
        delete next[uuid];
        return next;
      });
    }
  }

  function toggleSelectMode() {
    setSelectMode((v) => !v);
    setSelected(new Set());
  }

  function toggleSelected(uuid: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(uuid)) next.delete(uuid);
      else next.add(uuid);
      return next;
    });
  }

  async function runBulk(label: string, action: (uuid: string) => Promise<unknown>, confirmMsg?: string) {
    if (selected.size === 0) return;
    if (confirmMsg && !window.confirm(confirmMsg)) return;
    setBulkBusy(true);
    setBulkError(null);
    const uuids = Array.from(selected);
    const results = await Promise.allSettled(uuids.map((uuid) => action(uuid)));
    const failed = results.filter((r) => r.status === 'rejected').length;
    setBulkBusy(false);
    if (failed > 0) {
      setBulkError(
        t('serverList.bulkResultError', { label, succeeded: uuids.length - failed, total: uuids.length, failed }),
      );
    }
    setSelected(new Set());
  }

  const bulkPower = (action: PowerAction, confirmMsg?: string) =>
    runBulk(t(POWER_LABEL_KEYS[action]), (uuid) => api.power(uuid, action), confirmMsg);

  function bulkBackup() {
    const name = `bulk-${new Date().toISOString().replace(/[:.]/g, '-')}`;
    return runBulk(t('serverList.backup'), (uuid) => api.createServerBackup(uuid, name, []));
  }

  function bulkSuspend(suspend: boolean) {
    return runBulk(
      suspend ? t('serverView.suspend') : t('serverView.unsuspend'),
      (uuid) => (suspend ? api.suspendServer(uuid) : api.unsuspendServer(uuid)),
      suspend ? t('serverList.confirmBulkSuspend', { count: selected.size }) : undefined,
    );
  }

  if (loading) return <p className="srv-desc">{t('serverList.loadingServers')}</p>;
  if (error) return <div className="login-error show">{error}</div>;

  return (
    <div>
      <div className="dash-stats">
        <div className="stat-card">
          <div className="stat-card-val">{stats.total}</div>
          <div className="stat-card-lbl">{t('serverList.statServers')}</div>
        </div>
        <div className="stat-card">
          <div className="stat-card-val">{stats.online}</div>
          <div className="stat-card-lbl">{t('serverList.statOnline')}</div>
        </div>
        <div className="stat-card">
          <div className="stat-card-val">{stats.offline}</div>
          <div className="stat-card-lbl">{t('serverList.statOffline')}</div>
        </div>
      </div>

      <div className="dash-toolbar">
        <div className="search-wrap">
          <span className="search-icon">⌕</span>
          <input
            type="text"
            placeholder={t('serverList.searchPlaceholder')}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <button className="btn-sm" onClick={toggleSelectMode}>
          {selectMode ? t('serverList.cancelSelection') : t('serverList.selectServers')}
        </button>
      </div>

      {bulkError && <div className="login-error show" style={{ marginBottom: 16 }}>{bulkError}</div>}

      {selectMode && selected.size > 0 && (
        <div className="dash-toolbar" style={{ flexWrap: 'wrap', gap: 8 }}>
          <span className="srv-desc">{t('serverList.selectedCount', { count: selected.size })}</span>
          <button className="btn-sm" disabled={bulkBusy} onClick={() => bulkPower('start')}>
            {t('serverView.start')}
          </button>
          <button className="btn-sm" disabled={bulkBusy} onClick={() => bulkPower('stop')}>
            {t('serverView.stop')}
          </button>
          <button
            className="btn-sm"
            disabled={bulkBusy}
            onClick={() => bulkPower('restart', t('serverList.confirmRestart', { count: selected.size }))}
          >
            {t('serverView.restart')}
          </button>
          <button className="btn-sm" disabled={bulkBusy} onClick={bulkBackup}>
            {t('serverList.backup')}
          </button>
          {isAdmin && (
            <>
              <button className="btn-sm" disabled={bulkBusy} onClick={() => bulkSuspend(true)}>
                {t('serverView.suspend')}
              </button>
              <button className="btn-sm" disabled={bulkBusy} onClick={() => bulkSuspend(false)}>
                {t('serverView.unsuspend')}
              </button>
            </>
          )}
        </div>
      )}

      <div className="servers-grid">
        {filtered.map((server) => (
          <ServerCard
            key={server.uuid}
            server={server}
            onManage={onManage}
            onPower={handlePower}
            pendingAction={pendingActions[server.uuid] ?? null}
            selectable={selectMode}
            selected={selected.has(server.uuid)}
            onToggleSelect={toggleSelected}
          />
        ))}
        {filtered.length === 0 && <p className="srv-desc">{t('serverList.noServersMatch')}</p>}
      </div>
    </div>
  );
}
