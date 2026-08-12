import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import { GuidePanel } from '../components/GuidePanel';
import { useStreamSignal } from '../hooks/useStreamSignal';
import { useStreamSessionStats } from '../hooks/useStreamSessionStats';
import { formatDuration, formatRelativeTime } from '../utils/format';
import { obsServerUrlFor, relaySecretOf } from '../utils/streaming';
import type { Server } from '../types';

interface Props {
  onManage: (uuid: string) => void;
  onCreateStreaming: () => void;
}

export function Streamers({ onManage, onCreateStreaming }: Props) {
  const [servers, setServers] = useState<Server[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [twitchNotice, setTwitchNotice] = useState<'connected' | 'error' | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([api.listServers(), api.listEggs()])
      .then(async ([allServers, eggs]) => {
        const streamingEggIds = new Set(eggs.filter((e) => e.category === 'streaming').map((e) => e.id));
        const candidates = allServers.filter((s) => streamingEggIds.has(s.egg_id));
        // listServers doesn't include environment (it's per-row secret data,
        // not something worth carrying for every server in a list response);
        // fetch full details for just the handful of streaming servers so
        // their OBS credentials can be shown right here.
        const full = await Promise.all(candidates.map((s) => api.getServer(s.uuid).catch(() => s)));
        if (!cancelled) setServers(full);
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

  // The OAuth round trip leaves the app entirely (redirects to twitch.tv and
  // back), so the result comes back as a query param on this page rather
  // than a normal API response. Show it once, then scrub it from the URL so
  // a refresh doesn't re-show a stale toast.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const result = params.get('twitch');
    if (result === 'connected' || result === 'error') {
      setTwitchNotice(result);
      params.delete('twitch');
      const rest = params.toString();
      window.history.replaceState(null, '', window.location.pathname + (rest ? `?${rest}` : ''));
    }
  }, []);

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>{t('streamers.title')}</h1>
        <p>{t('streamers.subtitle')}</p>
      </div>

      {error && <div className="login-error show" style={{ marginBottom: 16 }}>{error}</div>}

      <div className="servers-grid" style={{ marginBottom: 20 }}>
        <div className="settings-card tile-action" onClick={onCreateStreaming} style={{ cursor: 'pointer' }}>
          <div className="settings-card-title">{t('streamers.createTile')}</div>
          <p className="srv-desc">{t('streamers.createTileHint')}</p>
        </div>

        <TwitchTile notice={twitchNotice} onDismissNotice={() => setTwitchNotice(null)} />
      </div>

      <GuidePanel title={t('streamers.guideTile')}>
        <p>{t('streamers.guideP1')}</p>
        <p>{t('streamers.guideP2')}</p>
        <p>{t('streamers.guideP3')}</p>
      </GuidePanel>

      {loading ? (
        <p className="srv-desc">{t('common.loading')}</p>
      ) : servers.length === 0 ? (
        <p className="srv-desc" style={{ marginTop: 16 }}>
          {t('streamers.noServers')}
        </p>
      ) : (
        <div className="servers-grid" style={{ marginTop: 20 }}>
          {servers.map((server) => (
            <StreamerTile key={server.uuid} server={server} onManage={onManage} />
          ))}
        </div>
      )}
    </div>
  );
}

function StreamerTile({ server, onManage }: { server: Server; onManage: (uuid: string) => void }) {
  const { signalLive, inboundKbps, liveSince } = useStreamSignal(server.uuid);
  const { lastSession, currentPeakKbps, currentAvgKbps } = useStreamSessionStats(
    server.uuid,
    signalLive,
    liveSince,
    inboundKbps,
  );
  const secret = relaySecretOf(server);
  const serverUrl = obsServerUrlFor(server);

  return (
    <div className="settings-card">
      <div
        className="settings-card-title"
        style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}
      >
        <span>{server.name}</span>
        <div className={`status-badge ${signalLive ? 'online' : 'offline'}`}>
          <span className="dot" />
          {signalLive ? t('serverView.signalLive') : t('serverView.signalNone')}
        </div>
      </div>

      {signalLive ? (
        <p className="srv-desc" style={{ marginBottom: 10 }}>
          {t('streamers.statsNow', { kbps: inboundKbps, peak: currentPeakKbps, avg: currentAvgKbps })}
          {liveSince ? ` · ${formatDuration((Date.now() - liveSince) / 1000)}` : ''}
        </p>
      ) : (
        lastSession && (
          <p className="srv-desc" style={{ marginBottom: 10 }}>
            {t('streamers.statsLast', {
              ago: formatRelativeTime(lastSession.endedAt),
              duration: formatDuration((lastSession.endedAt - lastSession.startedAt) / 1000),
              peak: lastSession.peakKbps,
              avg: lastSession.avgKbps,
            })}
          </p>
        )
      )}

      {serverUrl && secret ? (
        <>
          <div className="sfield" style={{ marginBottom: 10 }}>
            <label>{t('serverView.obsServer')}</label>
            <div className="api-item">
              <span className="api-key">{serverUrl}</span>
              <button className="btn-sm" onClick={() => navigator.clipboard?.writeText(serverUrl)}>
                {t('common.copy')}
              </button>
            </div>
          </div>
          <div className="sfield" style={{ marginBottom: 10 }}>
            <label>{t('serverView.obsStreamKey')}</label>
            <div className="api-item">
              <span className="api-key">{secret}</span>
              <button className="btn-sm" onClick={() => navigator.clipboard?.writeText(secret)}>
                {t('common.copy')}
              </button>
            </div>
          </div>
        </>
      ) : (
        <p className="srv-desc" style={{ marginBottom: 10 }}>
          {t('serverView.needsPort')}
        </p>
      )}

      <div className="settings-foot">
        <button className="btn-sm" onClick={() => onManage(server.uuid)}>
          {t('streamers.openServer')}
        </button>
      </div>
    </div>
  );
}

function TwitchTile({
  notice,
  onDismissNotice,
}: {
  notice: 'connected' | 'error' | null;
  onDismissNotice: () => void;
}) {
  const [status, setStatus] = useState<{ enabled: boolean; connected: boolean; twitch_login?: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function refresh() {
    api
      .getTwitchStatus()
      .then(setStatus)
      .catch(() => setStatus({ enabled: false, connected: false }));
  }

  useEffect(refresh, [notice]);

  async function connect() {
    setBusy(true);
    setError(null);
    try {
      const { authorize_url } = await api.startTwitchConnect();
      window.location.href = authorize_url;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  async function disconnect() {
    setBusy(true);
    setError(null);
    try {
      await api.disconnectTwitch();
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="settings-card">
      <div className="settings-card-title">{t('streamers.servicesTile')}</div>

      {notice === 'connected' && (
        <p className="srv-desc" style={{ color: 'var(--green, #5fe69a)', marginBottom: 10 }} onClick={onDismissNotice}>
          {t('streamers.twitchJustConnected')}
        </p>
      )}
      {notice === 'error' && (
        <p className="login-error show" style={{ marginBottom: 10 }} onClick={onDismissNotice}>
          {t('streamers.twitchConnectError')}
        </p>
      )}
      {error && <p className="login-error show" style={{ marginBottom: 10 }}>{error}</p>}

      {status === null ? (
        <p className="srv-desc">{t('common.loading')}</p>
      ) : !status.enabled ? (
        <p className="srv-desc">{t('streamers.twitchNotConfigured')}</p>
      ) : status.connected ? (
        <>
          <p className="srv-desc" style={{ marginBottom: 10 }}>
            {t('streamers.twitchConnectedAs', { login: status.twitch_login ?? '' })}
          </p>
          <div className="settings-foot">
            <button className="btn-sm" disabled={busy} onClick={disconnect}>
              {t('streamers.twitchDisconnect')}
            </button>
          </div>
        </>
      ) : (
        <>
          <p className="srv-desc" style={{ marginBottom: 10 }}>
            {t('streamers.twitchConnectHint')}
          </p>
          <button className="btn-sm" disabled={busy} onClick={connect}>
            {t('streamers.twitchConnect')}
          </button>
        </>
      )}
    </div>
  );
}
