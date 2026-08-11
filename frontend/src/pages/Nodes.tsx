import { useEffect, useMemo, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import type { Allocation, CreateNodeResponse, DatabaseHost, Node, NodeStatus } from '../types';

const INSTALL_SCRIPT_URL = 'https://raw.githubusercontent.com/superbodik/PowerNode/main/install.sh';

function nodeInstallCommand(daemonToken: string): string {
  return `WINGSD_DAEMON_TOKEN=${daemonToken} WINGSD_PANEL_URL=${window.location.origin} bash <(curl -sSL ${INSTALL_SCRIPT_URL})`;
}

export function Nodes() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [justCreated, setJustCreated] = useState<CreateNodeResponse | null>(null);

  const [form, setForm] = useState({
    name: '',
    fqdn: '',
    scheme: 'http',
    location_id: 1,
    memory_mb: 8192,
    disk_mb: 102400,
  });
  const [submitting, setSubmitting] = useState(false);

  const [allocationNodeId, setAllocationNodeId] = useState(0);
  const [allocations, setAllocations] = useState<Allocation[]>([]);
  const [allocForm, setAllocForm] = useState({ ip: '', port: 25565, portEnd: 25565 });
  const [allocError, setAllocError] = useState<string | null>(null);
  const [allocSubmitting, setAllocSubmitting] = useState(false);

  const [dbHosts, setDbHosts] = useState<DatabaseHost[]>([]);
  const [dbHostForm, setDbHostForm] = useState({
    name: '',
    host: '',
    port: 3306,
    admin_username: 'root',
    admin_password: '',
  });
  const [dbHostError, setDbHostError] = useState<string | null>(null);
  const [dbHostSubmitting, setDbHostSubmitting] = useState(false);

  function refreshDbHosts() {
    api.listDatabaseHosts().then(setDbHosts).catch(() => {});
  }

  async function handleCreateDbHost(e: React.FormEvent) {
    e.preventDefault();
    setDbHostSubmitting(true);
    setDbHostError(null);
    try {
      await api.createDatabaseHost(dbHostForm);
      setDbHostForm((f) => ({ ...f, name: '', host: '', admin_password: '' }));
      refreshDbHosts();
    } catch (err) {
      setDbHostError(err instanceof Error ? err.message : String(err));
    } finally {
      setDbHostSubmitting(false);
    }
  }

  async function handleDeleteDbHost(id: number) {
    if (!window.confirm(t('nodes.confirmDeleteDbHost'))) return;
    try {
      await api.deleteDatabaseHost(id);
      refreshDbHosts();
    } catch (err) {
      setDbHostError(err instanceof Error ? err.message : String(err));
    }
  }

  const [statuses, setStatuses] = useState<Record<number, NodeStatus | 'checking'>>({});
  const [expandedNodeId, setExpandedNodeId] = useState<number | null>(null);
  const [deletingNodeId, setDeletingNodeId] = useState<number | null>(null);
  const [panelVersion, setPanelVersion] = useState<string | null>(null);
  const [query, setQuery] = useState('');

  useEffect(() => {
    api.getVersion().then((v) => setPanelVersion(v.version)).catch(() => {});
  }, []);

  const filteredNodes = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return nodes;
    return nodes.filter((n) => n.name.toLowerCase().includes(q) || n.fqdn.toLowerCase().includes(q));
  }, [nodes, query]);
  const [editForm, setEditForm] = useState({
    name: '',
    fqdn: '',
    scheme: 'http',
    daemon_port: 8443,
    memory_mb: 0,
    memory_overallocate: 0,
    disk_mb: 0,
    disk_overallocate: 0,
    is_public: true,
    maintenance_mode: false,
  });
  const [savingNodeId, setSavingNodeId] = useState<number | null>(null);
  const [regeneratingNodeId, setRegeneratingNodeId] = useState<number | null>(null);
  const [regeneratedToken, setRegeneratedToken] = useState<CreateNodeResponse | null>(null);

  function toggleExpand(node: Node) {
    if (expandedNodeId === node.id) {
      setExpandedNodeId(null);
      return;
    }
    setExpandedNodeId(node.id);
    setRegeneratedToken(null);
    setEditForm({
      name: node.name,
      fqdn: node.fqdn,
      scheme: node.scheme,
      daemon_port: node.daemon_port,
      memory_mb: node.memory_mb,
      memory_overallocate: node.memory_overallocate,
      disk_mb: node.disk_mb,
      disk_overallocate: node.disk_overallocate,
      is_public: node.is_public,
      maintenance_mode: node.maintenance_mode,
    });
  }

  async function handleRegenerateToken(node: Node) {
    if (!window.confirm(t('nodes.confirmRegenToken', { name: node.name }))) {
      return;
    }
    setRegeneratingNodeId(node.id);
    setError(null);
    try {
      setRegeneratedToken(await api.regenerateNodeToken(node.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setRegeneratingNodeId(null);
    }
  }

  async function handleSaveNode(node: Node) {
    setSavingNodeId(node.id);
    setError(null);
    try {
      await api.updateNode(node.id, editForm);
      setExpandedNodeId(null);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSavingNodeId(null);
    }
  }

  async function handleDeleteNode(node: Node) {
    if (!window.confirm(t('nodes.confirmDeleteNode', { name: node.name }))) {
      return;
    }
    setDeletingNodeId(node.id);
    setError(null);
    try {
      await api.deleteNode(node.id);
      setExpandedNodeId(null);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setDeletingNodeId(null);
    }
  }

  async function handleCheckStatus(nodeId: number) {
    setStatuses((s) => ({ ...s, [nodeId]: 'checking' }));
    try {
      const status = await api.checkNodeStatus(nodeId);
      setStatuses((s) => ({ ...s, [nodeId]: status }));
    } catch (err) {
      setStatuses((s) => ({
        ...s,
        [nodeId]: { online: false, error: err instanceof Error ? err.message : String(err) },
      }));
    }
  }

  function refreshAllocations(nodeId: number) {
    if (!nodeId) {
      setAllocations([]);
      return;
    }
    api
      .listAllocations(nodeId)
      .then(setAllocations)
      .catch(() => setAllocations([]));
  }

  useEffect(() => {
    refreshAllocations(allocationNodeId);
  }, [allocationNodeId]);

  async function handleCreateAllocation(e: React.FormEvent) {
    e.preventDefault();
    setAllocSubmitting(true);
    setAllocError(null);
    try {
      const result = await api.createAllocation({
        node_id: allocationNodeId,
        ip: allocForm.ip,
        port: allocForm.port,
        port_end: allocForm.portEnd,
      });
      const next = allocForm.portEnd + 1;
      setAllocForm((f) => ({ ...f, port: next, portEnd: next }));
      if (result.created < allocForm.portEnd - allocForm.port + 1) {
        setAllocError(
          t('nodes.addedPortsPartial', {
            created: result.created,
            total: allocForm.portEnd - allocForm.port + 1,
          }),
        );
      }
      refreshAllocations(allocationNodeId);
    } catch (err) {
      setAllocError(err instanceof Error ? err.message : String(err));
    } finally {
      setAllocSubmitting(false);
    }
  }

  async function handleDeleteAllocation(id: number) {
    try {
      await api.deleteAllocation(id);
      refreshAllocations(allocationNodeId);
    } catch (err) {
      setAllocError(err instanceof Error ? err.message : String(err));
    }
  }

  function refresh() {
    setLoading(true);
    api
      .listNodes()
      .then(setNodes)
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, []);
  useEffect(refreshDbHosts, []);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const created = await api.createNode(form);
      setJustCreated(created);
      setForm((f) => ({ ...f, name: '', fqdn: '' }));
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>{t('nodes.title')}</h1>
        <p>{t('nodes.subtitle')}</p>
      </div>

      {justCreated && (
        <div className="acc-card" style={{ marginBottom: 20 }}>
          <div className="acc-card-title">{t('nodes.createdTitle')}</div>
          <p className="srv-desc" style={{ marginBottom: 10 }}>
            {t('nodes.createdHint')}
          </p>
          <div className="api-item">
            <span className="api-key">{nodeInstallCommand(justCreated.daemon_token)}</span>
            <button
              className="btn-sm"
              onClick={() => navigator.clipboard?.writeText(nodeInstallCommand(justCreated.daemon_token))}
            >
              {t('common.copy')}
            </button>
          </div>
          <p className="srv-desc" style={{ marginTop: 12, marginBottom: 6 }}>
            {t('nodes.rawTokenHint')}
          </p>
          <div className="api-item">
            <span className="api-key">{justCreated.daemon_token}</span>
            <button
              className="btn-sm"
              onClick={() => navigator.clipboard?.writeText(justCreated.daemon_token)}
            >
              {t('common.copy')}
            </button>
          </div>
          <div className="settings-foot">
            <button className="btn-sm" onClick={() => setJustCreated(null)}>
              {t('nodes.done')}
            </button>
          </div>
        </div>
      )}

      {error && <div className="login-error show" style={{ marginBottom: 16 }}>{error}</div>}

      <div className="settings-card" style={{ marginBottom: 24 }}>
        <div className="settings-card-title">{t('nodes.addNode')}</div>
        <form onSubmit={handleCreate}>
          <div className="settings-grid">
            <div className="sfield">
              <label htmlFor="node-name">{t('nodes.name')}</label>
              <input
                id="node-name"
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                placeholder={t('nodes.placeholderNodeName')}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="node-fqdn">{t('nodes.fqdnIp')}</label>
              <input
                id="node-fqdn"
                value={form.fqdn}
                onChange={(e) => setForm((f) => ({ ...f, fqdn: e.target.value }))}
                placeholder={t('nodes.placeholderNodeFqdn')}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="node-scheme">{t('nodes.wingsdScheme')}</label>
              <select
                id="node-scheme"
                value={form.scheme}
                onChange={(e) => setForm((f) => ({ ...f, scheme: e.target.value }))}
              >
                <option value="http">{t('nodes.schemeHttpOption')}</option>
                <option value="https">{t('nodes.schemeHttpsOption')}</option>
              </select>
            </div>
            <div className="sfield">
              <label htmlFor="node-memory">{t('nodes.memoryMb')}</label>
              <input
                id="node-memory"
                type="number"
                value={form.memory_mb}
                onChange={(e) => setForm((f) => ({ ...f, memory_mb: Number(e.target.value) }))}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="node-disk">{t('nodes.diskMb')}</label>
              <input
                id="node-disk"
                type="number"
                value={form.disk_mb}
                onChange={(e) => setForm((f) => ({ ...f, disk_mb: Number(e.target.value) }))}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="node-location">{t('nodes.locationId')}</label>
              <input
                id="node-location"
                type="number"
                value={form.location_id}
                onChange={(e) => setForm((f) => ({ ...f, location_id: Number(e.target.value) }))}
                required
              />
            </div>
          </div>
          <div className="settings-foot">
            <button className="btn-primary" type="submit" disabled={submitting} style={{ width: 'auto', padding: '10px 20px' }}>
              {submitting ? t('nodes.creatingNode') : t('nodes.createNode')}
            </button>
          </div>
        </form>
      </div>

      {!loading && (
        <div className="dash-toolbar">
          <div className="search-wrap">
            <span className="search-icon">⌕</span>
            <input
              type="text"
              placeholder={t('nodes.searchPlaceholder')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
        </div>
      )}

      {loading ? (
        <p className="srv-desc">{t('nodes.loading')}</p>
      ) : (
        <div className="db-table">
          <div className="db-head">
            <span>{t('nodes.name')}</span>
            <span>{t('nodes.colAddress')}</span>
            <span>{t('nodes.colMemoryDisk')}</span>
            <span>{t('nodes.colStatus')}</span>
          </div>
          {filteredNodes.map((node) => {
            const status = statuses[node.id];
            const expanded = expandedNodeId === node.id;
            return (
              <div key={node.id}>
                <div className="db-row">
                  <span
                    className="db-name"
                    style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8 }}
                    onClick={() => toggleExpand(node)}
                  >
                    {node.name}
                    {!node.is_public && (
                      <span className="srv-desc" style={{ fontSize: 10, border: '1px solid var(--border)', borderRadius: 4, padding: '1px 5px' }}>
                        {t('nodes.private')}
                      </span>
                    )}
                    {node.maintenance_mode && (
                      <span style={{ fontSize: 10, color: 'var(--yellow, #f0b232)', border: '1px solid var(--border)', borderRadius: 4, padding: '1px 5px' }}>
                        {t('nodes.maintenance')}
                      </span>
                    )}
                  </span>
                  <span className="db-pw">
                    {node.scheme}://{node.fqdn}:{node.daemon_port}
                  </span>
                  <span>{node.memory_mb} MB / {node.disk_mb} MB</span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
                    {status === 'checking' ? (
                      t('nodes.checking')
                    ) : status ? (
                      <span
                        title={
                          status.error ??
                          (status.agent_version && panelVersion && status.agent_version !== panelVersion
                            ? t('nodes.versionMismatch', { agent: status.agent_version, panel: panelVersion })
                            : '')
                        }
                        style={{
                          color: !status.online
                            ? '#f23f43'
                            : status.agent_version && panelVersion && status.agent_version !== panelVersion
                              ? 'var(--yellow, #f0b232)'
                              : 'var(--pink-b)',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}
                      >
                        {status.online
                          ? `${t('nodes.online')}${status.agent_version ?? node.agent_version ? ` · v${status.agent_version ?? node.agent_version}` : ''}`
                          : t('nodes.unreachable', { error: status.error ?? t('nodes.unknownError') })}
                      </span>
                    ) : (
                      node.agent_version ? t('nodes.lastSeen', { version: node.agent_version }) : t('nodes.unknown')
                    )}
                    <button
                      className="file-act-btn"
                      title={t('nodes.checkConnection')}
                      onClick={() => handleCheckStatus(node.id)}
                      style={{ flexShrink: 0 }}
                    >
                      ⟳
                    </button>
                  </span>
                </div>
                {expanded && (
                  <div style={{ padding: '14px 18px', borderBottom: '1px solid rgba(192,100,120,.06)' }}>
                    <div className="settings-grid" style={{ marginBottom: 14 }}>
                      <div className="sfield">
                        <label htmlFor={`edit-name-${node.id}`}>{t('nodes.name')}</label>
                        <input
                          id={`edit-name-${node.id}`}
                          value={editForm.name}
                          onChange={(e) => setEditForm((f) => ({ ...f, name: e.target.value }))}
                        />
                      </div>
                      <div className="sfield">
                        <label htmlFor={`edit-fqdn-${node.id}`}>{t('nodes.fqdnIp')}</label>
                        <input
                          id={`edit-fqdn-${node.id}`}
                          value={editForm.fqdn}
                          onChange={(e) => setEditForm((f) => ({ ...f, fqdn: e.target.value }))}
                        />
                      </div>
                      <div className="sfield">
                        <label htmlFor={`edit-scheme-${node.id}`}>{t('nodes.wingsdScheme')}</label>
                        <select
                          id={`edit-scheme-${node.id}`}
                          value={editForm.scheme}
                          onChange={(e) => setEditForm((f) => ({ ...f, scheme: e.target.value }))}
                        >
                          <option value="http">{t('nodes.editSchemeHttpOption')}</option>
                          <option value="https">{t('nodes.editSchemeHttpsOption')}</option>
                        </select>
                      </div>
                      <div className="sfield">
                        <label htmlFor={`edit-port-${node.id}`}>{t('nodes.daemonPort')}</label>
                        <input
                          id={`edit-port-${node.id}`}
                          type="number"
                          value={editForm.daemon_port}
                          onChange={(e) =>
                            setEditForm((f) => ({ ...f, daemon_port: Number(e.target.value) }))
                          }
                        />
                      </div>
                      <div className="sfield">
                        <label htmlFor={`edit-memory-${node.id}`}>{t('nodes.memoryMb')}</label>
                        <input
                          id={`edit-memory-${node.id}`}
                          type="number"
                          value={editForm.memory_mb}
                          onChange={(e) =>
                            setEditForm((f) => ({ ...f, memory_mb: Number(e.target.value) }))
                          }
                        />
                      </div>
                      <div className="sfield">
                        <label htmlFor={`edit-memory-overallocate-${node.id}`}>{t('nodes.memoryOverallocate')}</label>
                        <input
                          id={`edit-memory-overallocate-${node.id}`}
                          type="number"
                          value={editForm.memory_overallocate}
                          onChange={(e) =>
                            setEditForm((f) => ({ ...f, memory_overallocate: Number(e.target.value) }))
                          }
                        />
                      </div>
                      <div className="sfield">
                        <label htmlFor={`edit-disk-${node.id}`}>{t('nodes.diskMb')}</label>
                        <input
                          id={`edit-disk-${node.id}`}
                          type="number"
                          value={editForm.disk_mb}
                          onChange={(e) =>
                            setEditForm((f) => ({ ...f, disk_mb: Number(e.target.value) }))
                          }
                        />
                      </div>
                      <div className="sfield">
                        <label htmlFor={`edit-disk-overallocate-${node.id}`}>{t('nodes.diskOverallocate')}</label>
                        <input
                          id={`edit-disk-overallocate-${node.id}`}
                          type="number"
                          value={editForm.disk_overallocate}
                          onChange={(e) =>
                            setEditForm((f) => ({ ...f, disk_overallocate: Number(e.target.value) }))
                          }
                        />
                      </div>
                    </div>
                    <div style={{ display: 'flex', gap: 20, marginBottom: 14 }}>
                      <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
                        <div
                          className={`toggle-sw ${editForm.is_public ? 'on' : ''}`}
                          onClick={() => setEditForm((f) => ({ ...f, is_public: !f.is_public }))}
                        >
                          <div className="toggle-knob" />
                        </div>
                        {t('nodes.publicToggleLabel')}
                      </label>
                      <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
                        <div
                          className={`toggle-sw ${editForm.maintenance_mode ? 'on' : ''}`}
                          onClick={() => setEditForm((f) => ({ ...f, maintenance_mode: !f.maintenance_mode }))}
                        >
                          <div className="toggle-knob" />
                        </div>
                        {t('nodes.maintenanceToggleLabel')}
                      </label>
                    </div>
                    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                      <button
                        className="btn-primary"
                        style={{ width: 'auto', padding: '8px 16px' }}
                        disabled={savingNodeId === node.id}
                        onClick={() => handleSaveNode(node)}
                      >
                        {savingNodeId === node.id ? t('common.saving') : t('common.save')}
                      </button>
                      <button
                        className="btn-sm"
                        disabled={regeneratingNodeId === node.id}
                        onClick={() => handleRegenerateToken(node)}
                      >
                        {regeneratingNodeId === node.id ? t('nodes.generating') : t('nodes.regenerateToken')}
                      </button>
                      <button
                        className="btn-danger"
                        style={{ width: 'auto', padding: '8px 16px' }}
                        disabled={deletingNodeId === node.id}
                        onClick={() => handleDeleteNode(node)}
                      >
                        {deletingNodeId === node.id ? t('common.deleting') : t('nodes.deleteNode')}
                      </button>
                    </div>

                    {regeneratedToken && regeneratedToken.id === node.id && (
                      <div style={{ marginTop: 14 }}>
                        <p className="srv-desc" style={{ marginBottom: 8 }}>
                          {t('nodes.newTokenHint')}
                        </p>
                        <div className="api-item">
                          <span className="api-key">{nodeInstallCommand(regeneratedToken.daemon_token)}</span>
                          <button
                            className="btn-sm"
                            onClick={() =>
                              navigator.clipboard?.writeText(nodeInstallCommand(regeneratedToken.daemon_token))
                            }
                          >
                            {t('common.copy')}
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            );
          })}
          {nodes.length === 0 && <p className="srv-desc" style={{ padding: 16 }}>{t('nodes.noNodesYet')}</p>}
          {nodes.length > 0 && filteredNodes.length === 0 && (
            <p className="srv-desc" style={{ padding: 16 }}>{t('nodes.noNodesMatch')}</p>
          )}
        </div>
      )}

      <div className="settings-card" style={{ marginTop: 24 }}>
        <div className="settings-card-title">{t('nodes.allocationsTitle')}</div>
        <div className="settings-grid" style={{ marginBottom: 16 }}>
          <div className="sfield">
            <label htmlFor="alloc-node">{t('nodes.selectNodeLabel')}</label>
            <select
              id="alloc-node"
              value={allocationNodeId}
              onChange={(e) => setAllocationNodeId(Number(e.target.value))}
            >
              <option value={0} disabled>
                {t('nodes.selectNodePlaceholder')}
              </option>
              {nodes.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        {allocationNodeId > 0 && (
          <>
            <form onSubmit={handleCreateAllocation}>
              <div className="settings-grid">
                <div className="sfield">
                  <label htmlFor="alloc-ip">{t('nodes.ip')}</label>
                  <input
                    id="alloc-ip"
                    value={allocForm.ip}
                    onChange={(e) => setAllocForm((f) => ({ ...f, ip: e.target.value }))}
                    placeholder={t('nodes.placeholderAllocIp')}
                    required
                  />
                </div>
                <div className="sfield">
                  <label htmlFor="alloc-port">{t('nodes.portStart')}</label>
                  <input
                    id="alloc-port"
                    type="number"
                    value={allocForm.port}
                    onChange={(e) => {
                      const port = Number(e.target.value);
                      setAllocForm((f) => ({
                        ...f,
                        port,
                        portEnd: f.portEnd === f.port ? port : f.portEnd,
                      }));
                    }}
                    required
                  />
                </div>
                <div className="sfield">
                  <label htmlFor="alloc-port-end">{t('nodes.portEndOptional')}</label>
                  <input
                    id="alloc-port-end"
                    type="number"
                    value={allocForm.portEnd}
                    onChange={(e) => setAllocForm((f) => ({ ...f, portEnd: Number(e.target.value) }))}
                    required
                  />
                </div>
              </div>
              {allocError && <div className="login-error show" style={{ marginTop: 12 }}>{allocError}</div>}
              <div className="settings-foot">
                <button
                  className="btn-sm primary"
                  type="submit"
                  disabled={allocSubmitting}
                >
                  {allocSubmitting ? t('nodes.adding') : t('nodes.addAllocations')}
                </button>
              </div>
            </form>

            <div className="db-table" style={{ marginTop: 16 }}>
              <div className="db-head">
                <span>{t('nodes.colAddress')}</span>
                <span>{t('nodes.colStatus')}</span>
                <span />
                <span />
              </div>
              {allocations.map((a) => (
                <div className="db-row" key={a.id}>
                  <span className="db-name">
                    {a.ip}:{a.port}
                  </span>
                  <span>{a.server_id ? t('nodes.inUse') : t('nodes.free')}</span>
                  <span />
                  <span>
                    {!a.server_id && (
                      <button className="file-act-btn del" onClick={() => handleDeleteAllocation(a.id)}>
                        {t('common.delete')}
                      </button>
                    )}
                  </span>
                </div>
              ))}
              {allocations.length === 0 && (
                <p className="srv-desc" style={{ padding: 16 }}>
                  {t('nodes.noAllocationsYet')}
                </p>
              )}
            </div>
          </>
        )}
      </div>

      <div className="settings-card" style={{ marginTop: 24 }}>
        <div className="settings-card-title">{t('nodes.dbHostsTitle')}</div>
        <p className="srv-desc" style={{ marginBottom: 14 }}>
          {t('nodes.dbHostsHint')}
        </p>
        <form onSubmit={handleCreateDbHost}>
          <div className="settings-grid">
            <div className="sfield">
              <label htmlFor="dbhost-name">{t('nodes.name')}</label>
              <input
                id="dbhost-name"
                value={dbHostForm.name}
                onChange={(e) => setDbHostForm((f) => ({ ...f, name: e.target.value }))}
                placeholder={t('nodes.placeholderDbHostName')}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="dbhost-host">{t('nodes.host')}</label>
              <input
                id="dbhost-host"
                value={dbHostForm.host}
                onChange={(e) => setDbHostForm((f) => ({ ...f, host: e.target.value }))}
                placeholder={t('nodes.placeholderDbHost')}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="dbhost-port">{t('nodes.port')}</label>
              <input
                id="dbhost-port"
                type="number"
                value={dbHostForm.port}
                onChange={(e) => setDbHostForm((f) => ({ ...f, port: Number(e.target.value) }))}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="dbhost-user">{t('nodes.adminUsername')}</label>
              <input
                id="dbhost-user"
                value={dbHostForm.admin_username}
                onChange={(e) => setDbHostForm((f) => ({ ...f, admin_username: e.target.value }))}
                required
              />
            </div>
            <div className="sfield">
              <label htmlFor="dbhost-pass">{t('nodes.adminPassword')}</label>
              <input
                id="dbhost-pass"
                type="password"
                value={dbHostForm.admin_password}
                onChange={(e) => setDbHostForm((f) => ({ ...f, admin_password: e.target.value }))}
                required
              />
            </div>
          </div>
          {dbHostError && <div className="login-error show" style={{ marginTop: 12 }}>{dbHostError}</div>}
          <div className="settings-foot">
            <button className="btn-sm primary" type="submit" disabled={dbHostSubmitting}>
              {dbHostSubmitting ? t('nodes.adding') : t('nodes.addDbHost')}
            </button>
          </div>
        </form>

        <div className="db-table" style={{ marginTop: 16 }}>
          <div className="db-head">
            <span>{t('nodes.name')}</span>
            <span>{t('nodes.colAddress')}</span>
            <span>{t('nodes.colAdminUser')}</span>
            <span />
          </div>
          {dbHosts.map((host) => (
            <div className="db-row" key={host.id}>
              <span className="db-name">{host.name}</span>
              <span className="db-pw">{host.host}:{host.port}</span>
              <span>{host.admin_username}</span>
              <span>
                <button className="file-act-btn del" onClick={() => handleDeleteDbHost(host.id)}>
                  {t('common.delete')}
                </button>
              </span>
            </div>
          ))}
          {dbHosts.length === 0 && (
            <p className="srv-desc" style={{ padding: 16 }}>
              {t('nodes.noDbHostsYet')}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
