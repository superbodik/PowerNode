import { useEffect, useRef, useState } from 'react';
import { api, connectConsoleSocketWithRetry, connectServerSocketWithRetry } from '../api/client';
import type { ConsoleHandle } from '../api/client';
import { t } from '../i18n';
import { BackupManager } from '../components/BackupManager';
import { DatabaseManager } from '../components/DatabaseManager';
import { DomainManager } from '../components/DomainManager';
import { EggVariableField, eggVariableHint } from '../components/EggVariableField';
import { FileManager } from '../components/FileManager';
import { GuidePanel } from '../components/GuidePanel';
import { PortManager } from '../components/PortManager';
import { ScheduleManager } from '../components/ScheduleManager';
import { SubuserManager } from '../components/SubuserManager';
import { useStreamSignal } from '../hooks/useStreamSignal';
import { formatDuration } from '../utils/format';
import { obsServerUrlFor, relaySecretOf } from '../utils/streaming';
import type { Egg, PowerAction, ResourceStats, Server } from '../types';

interface Props {
  uuid: string;
  onBack: () => void;
}

type Tab = 'overview' | 'console' | 'files' | 'databases' | 'network' | 'domains' | 'backups' | 'schedules' | 'sharing';

const TAB_LABEL_KEYS: Record<Tab, Parameters<typeof t>[0]> = {
  overview: 'serverView.tabOverview',
  console: 'serverView.tabConsole',
  files: 'serverView.tabFiles',
  databases: 'serverView.tabDatabases',
  network: 'serverView.tabNetwork',
  domains: 'serverView.tabDomains',
  backups: 'serverView.tabBackups',
  schedules: 'serverView.tabSchedules',
  sharing: 'serverView.tabSharing',
};

function pct(used: number, limitMB: number): number {
  const limitBytes = limitMB * 1024 * 1024;
  if (!limitBytes) return 0;
  return Math.min(100, Math.round((used / limitBytes) * 100));
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 MB';
  const mb = bytes / (1024 * 1024);
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb.toFixed(0)} MB`;
}

function loadUsername(): string {
  try {
    const raw = localStorage.getItem('user');
    if (!raw) return 'yourusername';
    return (JSON.parse(raw) as { username: string }).username;
  } catch {
    return 'yourusername';
  }
}

// Mirrors mtprotoproxy's own TLS-mode secret format ("ee" + secret + hex of
// the UTF-8 TLS_DOMAIN bytes) so the panel can show a ready tg://proxy link
// without needing the container to print it anywhere first.
function hexEncodeUtf8(value: string): string {
  return Array.from(new TextEncoder().encode(value))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

function buildTelegramProxyLink(server: Server, secret: string): string {
  const host = server.primary_address?.split(':')[0] ?? '';
  const port = server.environment.PROXY_PORT || server.primary_address?.split(':')[1] || '443';
  const tlsDomain = server.environment.TLS_DOMAIN || 'www.google.com';
  const tlsSecret = 'ee' + secret + hexEncodeUtf8(tlsDomain);
  const params = new URLSearchParams({ server: host, port: String(port), secret: tlsSecret });
  return 'tg://proxy?' + params.toString();
}

export function ServerView({ uuid, onBack }: Props) {
  const [server, setServer] = useState<Server | null>(null);
  const [live, setLive] = useState<ResourceStats | null>(null);
  const [tab, setTab] = useState<Tab>('overview');
  const [error, setError] = useState<string | null>(null);
  const [consoleLines, setConsoleLines] = useState<string[]>([]);
  const [consoleConnected, setConsoleConnected] = useState(false);
  const [command, setCommand] = useState('');
  const [deleting, setDeleting] = useState(false);
  const [suspending, setSuspending] = useState(false);
  const [pendingAction, setPendingAction] = useState<PowerAction | null>(null);
  const [editingInfo, setEditingInfo] = useState(false);
  const [savingInfo, setSavingInfo] = useState(false);
  const [infoError, setInfoError] = useState<string | null>(null);
  const [infoForm, setInfoForm] = useState({
    name: '',
    description: '',
    docker_image: '',
    startup_command: '',
    memory_mb: 0,
    disk_mb: 0,
    cpu_cores: '', // empty = unlimited; string so the field can be blank
    environment: {} as Record<string, string>,
  });
  const [eggs, setEggs] = useState<Egg[]>([]);
  const consoleRef = useRef<ConsoleHandle | null>(null);
  const outputRef = useRef<HTMLDivElement>(null);

  function refreshServer() {
    api
      .getServer(uuid)
      .then(setServer)
      .catch((err) => setError(err instanceof Error ? err.message : String(err)));
  }

  useEffect(refreshServer, [uuid]);

  useEffect(() => {
    api.listEggs().then(setEggs).catch(() => {});
  }, []);

  useEffect(() => connectServerSocketWithRetry<ResourceStats>(uuid, setLive), [uuid]);

  const { inboundKbps, signalLive, liveSince } = useStreamSignal(uuid);

  useEffect(() => {
    if (tab !== 'console') return;
    setConsoleLines([]);
    setConsoleConnected(false);
    const handle = connectConsoleSocketWithRetry(
      uuid,
      (line) => setConsoleLines((prev) => [...prev.slice(-500), line]),
      setConsoleConnected,
    );
    consoleRef.current = handle;
    return () => {
      handle.close();
      consoleRef.current = null;
    };
  }, [uuid, tab]);

  useEffect(() => {
    outputRef.current?.scrollTo({ top: outputRef.current.scrollHeight });
  }, [consoleLines]);

  async function handlePower(action: PowerAction) {
    if (pendingAction) return;
    setPendingAction(action);
    setError(null);
    try {
      await api.power(uuid, action);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPendingAction(null);
    }
  }

  function sendCommand() {
    if (!command.trim() || !consoleConnected || !consoleRef.current) return;
    consoleRef.current.send(command);
    setCommand('');
  }

  async function handleDelete() {
    if (!window.confirm(t('serverView.confirmDelete', { name: server?.name ?? '' }))) {
      return;
    }
    setDeleting(true);
    try {
      await api.deleteServer(uuid);
      onBack();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setDeleting(false);
    }
  }

  async function handleSuspendToggle() {
    if (!server) return;
    const willSuspend = !server.is_suspended;
    if (willSuspend && !window.confirm(t('serverView.confirmSuspend', { name: server.name }))) {
      return;
    }
    setSuspending(true);
    try {
      if (willSuspend) {
        await api.suspendServer(uuid);
      } else {
        await api.unsuspendServer(uuid);
      }
      refreshServer();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSuspending(false);
    }
  }

  function startEditInfo() {
    if (!server) return;
    setInfoForm({
      name: server.name,
      description: server.description ?? '',
      docker_image: server.docker_image,
      startup_command: server.startup_command,
      memory_mb: server.memory_mb,
      disk_mb: server.disk_mb,
      cpu_cores: server.cpu_percent ? String(server.cpu_percent / 100) : '',
      environment: { ...(server.environment ?? {}) },
    });
    setInfoError(null);
    setEditingInfo(true);
  }

  async function handleSaveInfo(e: React.FormEvent) {
    e.preventDefault();
    if (!server) return;
    setSavingInfo(true);
    setInfoError(null);
    try {
      const { cpu_cores, ...rest } = infoForm;
      const cores = parseFloat(cpu_cores);
      await api.updateServer(uuid, {
        ...rest,
        swap_mb: server.swap_mb,
        cpu_percent: Number.isFinite(cores) && cores > 0 ? Math.round(cores * 100) : null,
      });
      setEditingInfo(false);
      refreshServer();
    } catch (err) {
      setInfoError(err instanceof Error ? err.message : String(err));
    } finally {
      setSavingInfo(false);
    }
  }

  const eggVariables = server ? (eggs.find((e) => e.id === server.egg_id)?.variables ?? []) : [];
  const streamingEgg = server ? eggs.find((e) => e.id === server.egg_id)?.category === 'streaming' : false;
  const relaySecret = relaySecretOf(server);
  const obsServerUrl = obsServerUrlFor(server);
  const telegramProxyEgg = server ? eggs.find((e) => e.id === server.egg_id)?.name === 'Telegram MTProto Proxy' : false;
  const [proxyKeys, setProxyKeys] = useState<{ name: string; secret: string }[] | null>(null);
  const [proxyKeysMtime, setProxyKeysMtime] = useState<number | undefined>(undefined);
  const [legacySecret, setLegacySecret] = useState<string | null>(null);
  const [newKeyName, setNewKeyName] = useState('');
  const [keyBusy, setKeyBusy] = useState(false);
  const [keyError, setKeyError] = useState<string | null>(null);

  function loadProxyKeys(uuid: string) {
    api
      .readFile(uuid, 'keys.json')
      .then(({ text, mtime }) => {
        try {
          const parsed = JSON.parse(text) as { name: string; secret: string }[];
          setProxyKeys(parsed);
          setProxyKeysMtime(mtime);
        } catch {
          setProxyKeys(null);
        }
      })
      .catch(() => setProxyKeys(null));
  }

  // keys.json (multiple named secrets sharing one port) is what current
  // installs generate on boot -- but a server created before that egg
  // update, or one that hasn't started even once yet, won't have it. The
  // single PROXY_SECRET env var / .secret file fallback keeps those
  // showing *something* instead of an empty card.
  useEffect(() => {
    if (!telegramProxyEgg || !server) {
      setProxyKeys(null);
      setLegacySecret(null);
      return;
    }
    loadProxyKeys(server.uuid);
    if (server.environment.PROXY_SECRET) {
      setLegacySecret(server.environment.PROXY_SECRET);
    } else {
      api
        .readFile(server.uuid, '.secret')
        .then(({ text }) => setLegacySecret(text.trim()))
        .catch(() => setLegacySecret(null));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [telegramProxyEgg, server?.uuid, server?.environment.PROXY_SECRET]);

  async function addProxyKey() {
    if (!server) return;
    const name = newKeyName.trim();
    if (!name) return;
    if ((proxyKeys ?? []).some((k) => k.name === name)) {
      setKeyError(t('serverView.telegramProxyDuplicateName'));
      return;
    }
    setKeyBusy(true);
    setKeyError(null);
    try {
      const bytes = new Uint8Array(16);
      crypto.getRandomValues(bytes);
      const secret = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
      const updated = [...(proxyKeys ?? []), { name, secret }];
      await api.writeFile(server.uuid, 'keys.json', JSON.stringify(updated), proxyKeysMtime);
      setNewKeyName('');
      await api.power(server.uuid, 'restart');
      loadProxyKeys(server.uuid);
    } catch (err) {
      setKeyError(err instanceof Error ? err.message : String(err));
    } finally {
      setKeyBusy(false);
    }
  }

  async function deleteProxyKey(name: string) {
    if (!server || !proxyKeys) return;
    setKeyBusy(true);
    setKeyError(null);
    try {
      const updated = proxyKeys.filter((k) => k.name !== name);
      await api.writeFile(server.uuid, 'keys.json', JSON.stringify(updated), proxyKeysMtime);
      await api.power(server.uuid, 'restart');
      loadProxyKeys(server.uuid);
    } catch (err) {
      setKeyError(err instanceof Error ? err.message : String(err));
    } finally {
      setKeyBusy(false);
    }
  }

  const telegramProxyLinks =
    telegramProxyEgg && server && server.primary_address
      ? proxyKeys
        ? proxyKeys.map((k) => ({ name: k.name, link: buildTelegramProxyLink(server, k.secret) }))
        : legacySecret
          ? [{ name: null, link: buildTelegramProxyLink(server, legacySecret) }]
          : []
      : [];

  if (error) return <div className="login-error show">{error}</div>;
  if (!server) return <p className="srv-desc">{t('common.loading')}</p>;

  const cpuPct = live ? Math.min(100, Math.round(live.cpu_percent)) : 0;
  const memPct = live ? pct(live.memory_bytes, server.memory_mb) : 0;
  const diskPct = live ? pct(live.disk_bytes, server.disk_mb) : 0;

  return (
    <div className="view active">
      <div className="server-head">
        <span className="bc-sep" onClick={onBack} style={{ cursor: 'pointer' }}>
          {t('serverView.back')}
        </span>
        <h1 style={{ marginTop: 8 }}>{server.name}</h1>
        <p>
          {server.uuid_short} · {server.docker_image}
        </p>
      </div>

      <div style={{ display: 'flex', gap: 24, alignItems: 'flex-start' }}>
        <div style={{ width: 220, flexShrink: 0 }}>
          <div className="power-grid">
            <button
              className="power-btn start"
              onClick={() => handlePower('start')}
              disabled={server.is_suspended || !!pendingAction}
              title={server.is_suspended ? t('serverView.suspendedTitle') : undefined}
            >
              {pendingAction === 'start' ? t('serverView.starting') : t('serverView.start')}
            </button>
            <button
              className="power-btn stop"
              onClick={() => handlePower('stop')}
              disabled={!!pendingAction}
            >
              {pendingAction === 'stop' ? t('serverView.stopping') : t('serverView.stop')}
            </button>
            <button
              className="power-btn"
              onClick={() => handlePower('restart')}
              disabled={server.is_suspended || !!pendingAction}
              title={server.is_suspended ? t('serverView.suspendedTitle') : undefined}
            >
              {pendingAction === 'restart' ? t('serverView.restarting') : t('serverView.restart')}
            </button>
            <button
              className="power-btn kill"
              onClick={() => handlePower('kill')}
              disabled={!!pendingAction}
            >
              {pendingAction === 'kill' ? t('serverView.killing') : t('serverView.kill')}
            </button>
          </div>
          {server.is_suspended && (
            <p className="srv-desc" style={{ marginTop: 8, marginBottom: 0 }}>
              {t('serverView.suspended')}
            </p>
          )}

          <div className="res-list">
            <div className="res-item">
              <div className="res-head">
                <span>{t('serverView.cpu')}</span>
                <span className="res-val">{live ? `${cpuPct}%` : '—'}</span>
              </div>
              <div className="res-bar">
                <div className="res-bar-fill" style={{ width: `${cpuPct}%` }} />
              </div>
            </div>
            <div className="res-item">
              <div className="res-head">
                <span>{t('serverView.ram')}</span>
                <span className="res-val">{live ? formatBytes(live.memory_bytes) : '—'}</span>
              </div>
              <div className="res-bar">
                <div className="res-bar-fill" style={{ width: `${memPct}%` }} />
              </div>
            </div>
            <div className="res-item">
              <div className="res-head">
                <span>{t('serverView.disk')}</span>
                <span className="res-val">{live ? formatBytes(live.disk_bytes) : '—'}</span>
              </div>
              <div className="res-bar">
                <div className="res-bar-fill" style={{ width: `${diskPct}%` }} />
              </div>
            </div>
          </div>
        </div>

        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="tab-bar">
            {(['overview', 'console', 'files', 'databases', 'network', 'domains', 'backups', 'schedules', 'sharing'] as Tab[]).map((tabKey) => (
              <div
                key={tabKey}
                className={`tab-btn ${tab === tabKey ? 'active' : ''}`}
                onClick={() => setTab(tabKey)}
              >
                {t(TAB_LABEL_KEYS[tabKey])}
              </div>
            ))}
          </div>

          <div className={`tab-panel ${tab === 'overview' ? 'active' : ''}`}>
            <GuidePanel title={t('guide.overview.title')}>
              <p>{t('guide.overview.p1')}</p>
              <p>{t('guide.overview.p2')}</p>
              <p>{t('guide.overview.p3')}</p>
              <p>{t('guide.overview.p4', { username: loadUsername(), 'server-id': server.uuid_short })}</p>
              <p>{t('guide.overview.p5')}</p>
            </GuidePanel>
            <div className="settings-card">
              <div className="settings-card-title" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                {t('serverView.serverInfo')}
                {!editingInfo && (
                  <button className="btn-sm" onClick={startEditInfo}>
                    {t('serverView.edit')}
                  </button>
                )}
              </div>

              {editingInfo ? (
                <form onSubmit={handleSaveInfo}>
                  <div className="settings-grid">
                    <div className="sfield">
                      <label htmlFor="edit-srv-name">{t('serverView.name')}</label>
                      <input
                        id="edit-srv-name"
                        value={infoForm.name}
                        onChange={(e) => setInfoForm((f) => ({ ...f, name: e.target.value }))}
                        required
                      />
                    </div>
                    <div className="sfield span2">
                      <label htmlFor="edit-srv-desc">{t('serverView.description')}</label>
                      <input
                        id="edit-srv-desc"
                        value={infoForm.description}
                        onChange={(e) => setInfoForm((f) => ({ ...f, description: e.target.value }))}
                      />
                    </div>
                    <div className="sfield span2">
                      <label htmlFor="edit-srv-image">{t('createServer.dockerImage')}</label>
                      <input
                        id="edit-srv-image"
                        value={infoForm.docker_image}
                        onChange={(e) => setInfoForm((f) => ({ ...f, docker_image: e.target.value }))}
                        required
                      />
                    </div>
                    <div className="sfield span2">
                      <label htmlFor="edit-srv-startup">{t('serverView.startupCommand')}</label>
                      <input
                        id="edit-srv-startup"
                        value={infoForm.startup_command}
                        onChange={(e) => setInfoForm((f) => ({ ...f, startup_command: e.target.value }))}
                      />
                    </div>
                    <div className="sfield">
                      <label htmlFor="edit-srv-memory">{t('createServer.memoryMb')}</label>
                      <input
                        id="edit-srv-memory"
                        type="number"
                        value={infoForm.memory_mb}
                        onChange={(e) => setInfoForm((f) => ({ ...f, memory_mb: Number(e.target.value) }))}
                        required
                      />
                    </div>
                    <div className="sfield">
                      <label htmlFor="edit-srv-disk">{t('createServer.diskMb')}</label>
                      <input
                        id="edit-srv-disk"
                        type="number"
                        value={infoForm.disk_mb}
                        onChange={(e) => setInfoForm((f) => ({ ...f, disk_mb: Number(e.target.value) }))}
                        required
                      />
                    </div>
                    <div className="sfield">
                      <label htmlFor="edit-srv-cpu">{t('createServer.cpuCores')}</label>
                      <input
                        id="edit-srv-cpu"
                        type="number"
                        step="0.25"
                        min="0"
                        placeholder={t('createServer.cpuCoresUnlimited')}
                        value={infoForm.cpu_cores}
                        onChange={(e) => setInfoForm((f) => ({ ...f, cpu_cores: e.target.value }))}
                      />
                    </div>
                  </div>
                  {eggVariables.length > 0 && (
                    <div className="settings-grid" style={{ marginTop: 14 }}>
                      {eggVariables.map((v) => (
                        <div className="sfield" key={v.env_variable}>
                          <label htmlFor={`edit-env-${v.env_variable}`}>{v.name}</label>
                          <EggVariableField
                            id={`edit-env-${v.env_variable}`}
                            variable={v}
                            value={infoForm.environment[v.env_variable] ?? ''}
                            onChange={(value) =>
                              setInfoForm((f) => ({
                                ...f,
                                environment: { ...f.environment, [v.env_variable]: value },
                              }))
                            }
                          />
                          {eggVariableHint(v.rules) && (
                            <span className="srv-desc" style={{ fontSize: 10 }}>
                              {eggVariableHint(v.rules)}
                            </span>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                  {infoError && <div className="login-error show" style={{ marginTop: 12 }}>{infoError}</div>}
                  <p className="srv-desc" style={{ marginTop: 8 }}>
                    {t('serverView.editHint')}
                  </p>
                  <div className="settings-foot" style={{ display: 'flex', gap: 8 }}>
                    <button
                      className="btn-primary"
                      type="submit"
                      disabled={savingInfo}
                      style={{ width: 'auto', padding: '10px 20px' }}
                    >
                      {savingInfo ? t('common.saving') : t('serverView.saveChanges')}
                    </button>
                    <button className="btn-sm" type="button" onClick={() => setEditingInfo(false)} disabled={savingInfo}>
                      {t('common.cancel')}
                    </button>
                  </div>
                </form>
              ) : (
                <div className="settings-grid">
                  <div className="sfield">
                    <label>{t('serverView.status')}</label>
                    <input readOnly value={live?.state ?? server.status} />
                  </div>
                  <div className="sfield">
                    <label>{t('serverView.node')}</label>
                    <input readOnly value={server.node_name ?? '—'} />
                  </div>
                  <div className="sfield">
                    <label>{t('serverView.address')}</label>
                    <input readOnly value={server.primary_address ?? t('serverView.noAllocation')} />
                  </div>
                  <div className="sfield">
                    <label>{t('createServer.dockerImage')}</label>
                    <input readOnly value={server.docker_image} />
                  </div>
                  <div className="sfield">
                    <label>{t('serverView.startupCommand')}</label>
                    <input readOnly value={server.startup_command} />
                  </div>
                  <div className="sfield">
                    <label>{t('serverView.memoryLimit')}</label>
                    <input readOnly value={`${server.memory_mb} MB`} />
                  </div>
                  <div className="sfield">
                    <label>{t('serverView.diskLimit')}</label>
                    <input readOnly value={`${server.disk_mb} MB`} />
                  </div>
                  <div className="sfield">
                    <label>{t('createServer.cpuCores')}</label>
                    <input
                      readOnly
                      value={server.cpu_percent ? `${server.cpu_percent / 100}` : t('createServer.cpuCoresUnlimited')}
                    />
                  </div>
                </div>
              )}
            </div>

            <div className="settings-card" style={{ marginTop: 20 }}>
              <div className="settings-card-title">{t('serverView.sftpAccess')}</div>
              <p className="srv-desc" style={{ marginBottom: 12 }}>
                {t('serverView.sftpHintPrefix')}
                <strong>{t('nav.account')}</strong>
                {t('serverView.sftpHintSuffix')}
              </p>
              <div className="settings-grid">
                <div className="sfield">
                  <label>{t('serverView.host')}</label>
                  <input readOnly value={server.primary_address?.split(':')[0] ?? t('serverView.noAllocation')} />
                </div>
                <div className="sfield">
                  <label>{t('serverView.port')}</label>
                  <input readOnly value="2022" />
                </div>
                <div className="sfield">
                  <label>{t('serverView.username')}</label>
                  <input readOnly value={`${loadUsername()}.${server.uuid_short}`} />
                </div>
              </div>
            </div>

            {streamingEgg && (
              <div className="settings-card" style={{ marginTop: 20 }}>
                <div className="settings-card-title">{t('serverView.streamStatus')}</div>
                <div className="settings-grid">
                  <div className="sfield">
                    <label>{t('serverView.streamSignal')}</label>
                    <div className={`status-badge ${signalLive ? 'online' : 'offline'}`}>
                      <span className="dot" />
                      {signalLive ? t('serverView.signalLive') : t('serverView.signalNone')}
                    </div>
                  </div>
                  <div className="sfield">
                    <label>{t('serverView.streamBitrate')}</label>
                    <input readOnly value={signalLive ? `${inboundKbps} kbps` : '—'} />
                  </div>
                  <div className="sfield">
                    <label>{t('serverView.streamUptime')}</label>
                    <input
                      readOnly
                      value={signalLive && liveSince ? formatDuration((Date.now() - liveSince) / 1000) : '—'}
                    />
                  </div>
                </div>
              </div>
            )}

            {streamingEgg && relaySecret && (
              <div className="settings-card" style={{ marginTop: 20 }}>
                <div className="settings-card-title">{t('serverView.streamToObs')}</div>
                <p className="srv-desc" style={{ marginBottom: 12 }}>
                  {t('serverView.streamToObsHint')}
                </p>
                {server.primary_address ? (
                  <>
                    <div className="sfield" style={{ marginBottom: 10 }}>
                      <label>{t('serverView.obsServer')}</label>
                      <div className="api-item">
                        <span className="api-key">{obsServerUrl}</span>
                        <button className="btn-sm" onClick={() => navigator.clipboard?.writeText(obsServerUrl ?? '')}>
                          {t('common.copy')}
                        </button>
                      </div>
                    </div>
                    <div className="sfield">
                      <label>{t('serverView.obsStreamKey')}</label>
                      <div className="api-item">
                        <span className="api-key">{relaySecret}</span>
                        <button className="btn-sm" onClick={() => navigator.clipboard?.writeText(relaySecret ?? '')}>
                          {t('common.copy')}
                        </button>
                      </div>
                    </div>
                  </>
                ) : (
                  <p className="srv-desc">{t('serverView.needsPort')}</p>
                )}
              </div>
            )}

            {telegramProxyEgg && (
              <div className="settings-card" style={{ marginTop: 20 }}>
                <div className="settings-card-title">{t('serverView.telegramProxyTitle')}</div>
                {!server.primary_address ? (
                  <p className="srv-desc">{t('serverView.needsPort')}</p>
                ) : telegramProxyLinks.length === 0 ? (
                  <p className="srv-desc">{t('serverView.telegramProxyNoSecret')}</p>
                ) : (
                  <>
                    <p className="srv-desc" style={{ marginBottom: 12 }}>
                      {t('serverView.telegramProxyHint')}
                    </p>
                    {telegramProxyLinks.map((k, i) => (
                      <div className="sfield" key={k.name ?? i} style={{ marginBottom: 10 }}>
                        <label>{k.name ?? t('serverView.telegramProxyLinkLabel')}</label>
                        <div className="api-item">
                          <span className="api-key">{k.link}</span>
                          <button className="btn-sm" onClick={() => navigator.clipboard?.writeText(k.link)}>
                            {t('common.copy')}
                          </button>
                          {proxyKeys && k.name && (
                            <button className="btn-sm" disabled={keyBusy} onClick={() => deleteProxyKey(k.name!)}>
                              {t('serverView.telegramProxyRevoke')}
                            </button>
                          )}
                        </div>
                      </div>
                    ))}

                    {proxyKeys && (
                      <div className="sfield" style={{ marginTop: 14 }}>
                        <label>{t('serverView.telegramProxyNewKeyLabel')}</label>
                        <div className="api-item">
                          <input
                            value={newKeyName}
                            onChange={(e) => setNewKeyName(e.target.value)}
                            placeholder={t('serverView.telegramProxyNewKeyPlaceholder')}
                          />
                          <button className="btn-sm" disabled={keyBusy || !newKeyName.trim()} onClick={addProxyKey}>
                            {t('serverView.telegramProxyCreateKey')}
                          </button>
                        </div>
                        <span className="srv-desc" style={{ fontSize: 10 }}>
                          {t('serverView.telegramProxyNewKeyHint')}
                        </span>
                      </div>
                    )}
                  </>
                )}
                {keyError && <p className="login-error show" style={{ marginTop: 10 }}>{keyError}</p>}
              </div>
            )}

            <div className="danger-card" style={{ marginTop: 20 }}>
              <div className="danger-row">
                <div className="danger-info">
                  <h3>{server.is_suspended ? t('serverView.unsuspendServer') : t('serverView.suspendServer')}</h3>
                  <p>
                    {server.is_suspended
                      ? t('serverView.allowStartAgain')
                      : t('serverView.suspendHint')}
                  </p>
                </div>
                <button className="btn-sm" onClick={handleSuspendToggle} disabled={suspending}>
                  {suspending ? t('serverView.working') : server.is_suspended ? t('serverView.unsuspend') : t('serverView.suspend')}
                </button>
              </div>
            </div>

            <div className="danger-card" style={{ marginTop: 20 }}>
              <div className="danger-row">
                <div className="danger-info">
                  <h3>{t('serverView.deleteServer')}</h3>
                  <p>{t('serverView.deleteServerHint')}</p>
                </div>
                <button className="btn-danger" onClick={handleDelete} disabled={deleting}>
                  {deleting ? t('common.deleting') : t('common.delete')}
                </button>
              </div>
            </div>
          </div>

          <div className={`tab-panel ${tab === 'console' ? 'active' : ''}`}>
            <GuidePanel title={t('guide.console.title')}>
              <p>{t('guide.console.p1')}</p>
              <p>{t('guide.console.p2')}</p>
              <p>{t('guide.console.p3')}</p>
            </GuidePanel>
            <div className="console-wrap">
              <div className="console-bar">
                <span className="console-dot r" />
                <span className="console-dot y" />
                <span className="console-dot g" />
                <span className="console-title">{server.name}</span>
                <span className={`console-status ${consoleConnected ? 'online' : ''}`}>
                  {consoleConnected ? t('serverView.connected') : t('serverView.connecting')}
                </span>
              </div>
              <div className="console-output" ref={outputRef}>
                {consoleLines.map((line, i) => (
                  <div className="con-line" key={i}>
                    <span className="con-msg">{line}</span>
                  </div>
                ))}
                {consoleLines.length === 0 && (
                  <div className="con-line">
                    <span className="con-msg">
                      {consoleConnected ? t('serverView.waitingForOutput') : t('serverView.connectingToNode')}
                    </span>
                  </div>
                )}
              </div>
              <div className="console-input-row">
                <span className="console-prompt">$</span>
                <input
                  className="console-input"
                  value={command}
                  onChange={(e) => setCommand(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') sendCommand();
                  }}
                  placeholder={consoleConnected ? t('serverView.typeCommand') : t('serverView.connecting')}
                  disabled={!consoleConnected}
                />
                <button className="console-send" onClick={sendCommand} disabled={!consoleConnected}>
                  {t('serverView.send')}
                </button>
              </div>
            </div>
          </div>

          <div className={`tab-panel ${tab === 'files' ? 'active' : ''}`}>
            {tab === 'files' && <FileManager uuid={uuid} />}
          </div>

          <div className={`tab-panel ${tab === 'databases' ? 'active' : ''}`}>
            {tab === 'databases' && <DatabaseManager uuid={uuid} />}
          </div>

          <div className={`tab-panel ${tab === 'network' ? 'active' : ''}`}>
            {tab === 'network' && <PortManager uuid={uuid} />}
          </div>

          <div className={`tab-panel ${tab === 'domains' ? 'active' : ''}`}>
            {tab === 'domains' && <DomainManager uuid={uuid} />}
          </div>

          <div className={`tab-panel ${tab === 'backups' ? 'active' : ''}`}>
            {tab === 'backups' && <BackupManager uuid={uuid} />}
          </div>

          <div className={`tab-panel ${tab === 'schedules' ? 'active' : ''}`}>
            {tab === 'schedules' && <ScheduleManager uuid={uuid} />}
          </div>

          <div className={`tab-panel ${tab === 'sharing' ? 'active' : ''}`}>
            {tab === 'sharing' && <SubuserManager uuid={uuid} />}
          </div>
        </div>
      </div>
    </div>
  );
}
