import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import { GuidePanel } from '../components/GuidePanel';
import { useStreamSignal } from '../hooks/useStreamSignal';
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

        <div className="settings-card tile-soon">
          <div className="settings-card-title">{t('streamers.servicesTile')}</div>
          <p className="srv-desc">{t('streamers.servicesTileHint')}</p>
        </div>
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
  const { signalLive, inboundKbps } = useStreamSignal(server.uuid);
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

      {signalLive && (
        <p className="srv-desc" style={{ marginBottom: 10 }}>
          {inboundKbps} kbps
        </p>
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

      <button className="btn-sm" onClick={() => onManage(server.uuid)}>
        {t('streamers.openServer')}
      </button>
    </div>
  );
}
