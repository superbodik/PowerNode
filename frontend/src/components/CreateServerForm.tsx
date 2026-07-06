import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Allocation, Egg, Node } from '../types';

interface Props {
  onCreated: () => void;
}

export function CreateServerForm({ onCreated }: Props) {
  const [open, setOpen] = useState(false);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [eggs, setEggs] = useState<Egg[]>([]);
  const [allocations, setAllocations] = useState<Allocation[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [form, setForm] = useState({
    name: '',
    node_id: 0,
    egg_id: 0,
    docker_image: '',
    startup_command: '',
    memory_mb: 1024,
    disk_mb: 5120,
    allocation_id: 0,
  });
  const [environment, setEnvironment] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!open) return;
    api.listNodes().then(setNodes).catch(() => {});
    api.listEggs().then(setEggs).catch(() => {});
  }, [open]);

  useEffect(() => {
    if (!form.node_id) {
      setAllocations([]);
      return;
    }
    api
      .listAllocations(form.node_id, true)
      .then(setAllocations)
      .catch(() => setAllocations([]));
  }, [form.node_id]);

  function selectEgg(eggId: number) {
    const egg = eggs.find((e) => e.id === eggId);
    setForm((f) => ({
      ...f,
      egg_id: eggId,
      docker_image: egg?.docker_image ?? f.docker_image,
      startup_command: egg?.startup_command ?? f.startup_command,
    }));
    setEnvironment(
      Object.fromEntries((egg?.variables ?? []).map((v) => [v.env_variable, v.default_value])),
    );
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.createServer({
        name: form.name,
        node_id: form.node_id,
        egg_id: form.egg_id,
        docker_image: form.docker_image,
        startup_command: form.startup_command,
        environment,
        memory_mb: form.memory_mb,
        swap_mb: 0,
        disk_mb: form.disk_mb,
        allocation_id: form.allocation_id || undefined,
      });
      setOpen(false);
      setForm((f) => ({ ...f, name: '', allocation_id: 0 }));
      setEnvironment({});
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  if (!open) {
    return (
      <button className="btn-sm primary" onClick={() => setOpen(true)} style={{ marginBottom: 16 }}>
        + Create Server
      </button>
    );
  }

  return (
    <div className="settings-card" style={{ marginBottom: 24 }}>
      <div className="settings-card-title">Create server</div>
      <form onSubmit={handleSubmit}>
        <div className="settings-grid">
          <div className="sfield">
            <label htmlFor="srv-name">Name</label>
            <input
              id="srv-name"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              required
            />
          </div>
          <div className="sfield">
            <label htmlFor="srv-node">Node</label>
            <select
              id="srv-node"
              value={form.node_id}
              onChange={(e) => setForm((f) => ({ ...f, node_id: Number(e.target.value) }))}
              required
            >
              <option value={0} disabled>
                Select a node…
              </option>
              {nodes.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name}
                </option>
              ))}
            </select>
          </div>
          <div className="sfield">
            <label htmlFor="srv-egg">Egg</label>
            <select
              id="srv-egg"
              value={form.egg_id}
              onChange={(e) => selectEgg(Number(e.target.value))}
              required
            >
              <option value={0} disabled>
                Select an egg…
              </option>
              {eggs.map((egg) => (
                <option key={egg.id} value={egg.id}>
                  {egg.name}
                </option>
              ))}
            </select>
          </div>
          <div className="sfield">
            <label htmlFor="srv-allocation">Allocation (optional)</label>
            <select
              id="srv-allocation"
              value={form.allocation_id}
              onChange={(e) => setForm((f) => ({ ...f, allocation_id: Number(e.target.value) }))}
            >
              <option value={0}>None</option>
              {allocations.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.ip}:{a.port}
                </option>
              ))}
            </select>
          </div>
          <div className="sfield span2">
            <label htmlFor="srv-image">Docker image</label>
            <input
              id="srv-image"
              value={form.docker_image}
              onChange={(e) => setForm((f) => ({ ...f, docker_image: e.target.value }))}
              required
            />
          </div>
          <div className="sfield span2">
            <label htmlFor="srv-startup">Startup command</label>
            <input
              id="srv-startup"
              value={form.startup_command}
              onChange={(e) => setForm((f) => ({ ...f, startup_command: e.target.value }))}
              placeholder="leave empty to use the image's own entrypoint"
            />
          </div>
          <div className="sfield">
            <label htmlFor="srv-memory">Memory (MB)</label>
            <input
              id="srv-memory"
              type="number"
              value={form.memory_mb}
              onChange={(e) => setForm((f) => ({ ...f, memory_mb: Number(e.target.value) }))}
              required
            />
          </div>
          <div className="sfield">
            <label htmlFor="srv-disk">Disk (MB)</label>
            <input
              id="srv-disk"
              type="number"
              value={form.disk_mb}
              onChange={(e) => setForm((f) => ({ ...f, disk_mb: Number(e.target.value) }))}
              required
            />
          </div>
        </div>

        {(eggs.find((e) => e.id === form.egg_id)?.variables ?? []).length > 0 && (
          <div className="settings-grid" style={{ marginTop: 14 }}>
            {eggs
              .find((e) => e.id === form.egg_id)
              ?.variables.map((v) => (
                <div className="sfield" key={v.env_variable}>
                  <label htmlFor={`env-${v.env_variable}`}>{v.name}</label>
                  <input
                    id={`env-${v.env_variable}`}
                    value={environment[v.env_variable] ?? ''}
                    onChange={(e) =>
                      setEnvironment((env) => ({ ...env, [v.env_variable]: e.target.value }))
                    }
                    disabled={!v.is_editable}
                  />
                </div>
              ))}
          </div>
        )}

        {error && <div className="login-error show" style={{ marginTop: 12 }}>{error}</div>}

        <div className="settings-foot" style={{ display: 'flex', gap: 8 }}>
          <button
            className="btn-primary"
            type="submit"
            disabled={submitting}
            style={{ width: 'auto', padding: '10px 20px' }}
          >
            {submitting ? 'Creating…' : 'Create'}
          </button>
          <button className="btn-sm" type="button" onClick={() => setOpen(false)}>
            Cancel
          </button>
        </div>
      </form>
    </div>
  );
}
