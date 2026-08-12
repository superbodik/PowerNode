import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import type { Egg, Node, Server } from '../types';

// v1 of the plugin store: a "plugin" is just an egg in this category,
// installed as an ordinary server on whichever node is flagged as the
// plugin host (Nodes.tsx). This reuses the entire existing egg/server/
// daemon pipeline instead of inventing a second orchestration path --
// the catalog is curated by us via migrations, same as any other egg.
const PLUGIN_CATEGORY = 'Plugins';

interface Props {
  onManage: (uuid: string) => void;
}

export function Plugins({ onManage }: Props) {
  const [eggs, setEggs] = useState<Egg[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [servers, setServers] = useState<Server[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [installingEggId, setInstallingEggId] = useState<number | null>(null);
  const [uninstallingUuid, setUninstallingUuid] = useState<string | null>(null);

  function refresh() {
    setLoading(true);
    setError(null);
    Promise.all([api.listEggs(), api.listNodes(), api.listServers()])
      .then(([e, n, s]) => {
        setEggs(e);
        setNodes(n);
        setServers(s);
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, []);

  const pluginEggs = eggs.filter((e) => e.category === PLUGIN_CATEGORY);
  const pluginHost = nodes.find((n) => n.is_plugin_host) ?? null;
  const installedByEggId = new Map(
    servers
      .filter((s) => pluginEggs.some((e) => e.id === s.egg_id))
      .map((s) => [s.egg_id, s] as const),
  );

  async function handleInstall(egg: Egg) {
    if (!pluginHost) return;
    setError(null);
    setInstallingEggId(egg.id);
    try {
      const environment = Object.fromEntries(egg.variables.map((v) => [v.env_variable, v.default_value]));
      await api.createServer({
        name: egg.name,
        node_id: pluginHost.id,
        egg_id: egg.id,
        docker_image: egg.docker_image,
        startup_command: egg.startup_command,
        environment,
        memory_mb: 512,
        swap_mb: 0,
        disk_mb: 1024,
        cpu_percent: null,
      });
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setInstallingEggId(null);
    }
  }

  async function handleUninstall(server: Server) {
    if (!window.confirm(t('plugins.confirmUninstall', { name: server.name }))) return;
    setError(null);
    setUninstallingUuid(server.uuid);
    try {
      await api.deleteServer(server.uuid);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setUninstallingUuid(null);
    }
  }

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>{t('plugins.title')}</h1>
        <p>{t('plugins.subtitle')}</p>
      </div>

      {!loading && !pluginHost && (
        <div className="login-error show" style={{ marginBottom: 16 }}>
          {t('plugins.noHostWarning')}
        </div>
      )}
      {error && <div className="login-error show" style={{ marginBottom: 16 }}>{error}</div>}

      {loading ? (
        <p className="srv-desc">{t('common.loading')}</p>
      ) : pluginEggs.length === 0 ? (
        <p className="srv-desc">{t('plugins.noneYet')}</p>
      ) : (
        <div className="servers-grid">
          {pluginEggs.map((egg) => {
            const installed = installedByEggId.get(egg.id);
            return (
              <div className="settings-card" key={egg.id}>
                <div className="settings-card-title">{egg.name}</div>
                <p className="srv-desc" style={{ marginBottom: 14 }}>
                  {egg.description}
                </p>
                {installed ? (
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
                    <div className={`status-badge ${installed.status === 'running' ? 'online' : 'offline'}`}>
                      <span className="dot" />
                      {t('plugins.installed')}
                    </div>
                    <button className="btn-sm" onClick={() => onManage(installed.uuid)}>
                      {t('plugins.manage')}
                    </button>
                    <button
                      className="btn-danger"
                      disabled={uninstallingUuid === installed.uuid}
                      onClick={() => handleUninstall(installed)}
                    >
                      {uninstallingUuid === installed.uuid ? t('common.deleting') : t('plugins.uninstall')}
                    </button>
                  </div>
                ) : (
                  <button
                    className="btn-primary"
                    style={{ width: 'auto', padding: '8px 16px' }}
                    disabled={!pluginHost || installingEggId === egg.id}
                    onClick={() => handleInstall(egg)}
                  >
                    {installingEggId === egg.id ? t('plugins.installing') : t('plugins.install')}
                  </button>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
