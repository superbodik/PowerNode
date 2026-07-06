import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { PanelUser } from '../types';

export function Users() {
  const [users, setUsers] = useState<PanelUser[] | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<number, { serverLimit: string }>>({});
  const [saving, setSaving] = useState<number | null>(null);

  function refresh() {
    api
      .listUsers()
      .then((u) => {
        setUsers(u);
        setForbidden(false);
        setDrafts(
          Object.fromEntries(
            u.map((user) => [user.id, { serverLimit: user.server_limit?.toString() ?? '' }]),
          ),
        );
      })
      .catch(() => {
        setUsers(null);
        setForbidden(true);
      });
  }

  useEffect(refresh, []);

  async function handleToggleAdmin(u: PanelUser) {
    setSaving(u.id);
    setError(null);
    try {
      await api.updateUser(u.id, {
        is_admin: !u.is_admin,
        is_active: u.is_active,
        server_limit: u.server_limit,
      });
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(null);
    }
  }

  async function handleToggleActive(u: PanelUser) {
    setSaving(u.id);
    setError(null);
    try {
      await api.updateUser(u.id, {
        is_admin: u.is_admin,
        is_active: !u.is_active,
        server_limit: u.server_limit,
      });
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(null);
    }
  }

  async function handleSaveLimit(u: PanelUser) {
    const raw = drafts[u.id]?.serverLimit ?? '';
    const serverLimit = raw.trim() === '' ? null : Number(raw);
    setSaving(u.id);
    setError(null);
    try {
      await api.updateUser(u.id, {
        is_admin: u.is_admin,
        is_active: u.is_active,
        server_limit: serverLimit,
      });
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(null);
    }
  }

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>Users</h1>
        <p>Everyone with a Roost account.</p>
      </div>

      {forbidden && <p className="srv-desc">Only admins can manage users.</p>}

      {error && <div className="login-error show" style={{ marginBottom: 16 }}>{error}</div>}

      {!forbidden && users === null && <p className="srv-desc">Loading…</p>}

      {users && (
        <div className="db-table">
          <div className="db-head">
            <span>User</span>
            <span>Admin</span>
            <span>Active</span>
            <span>Server limit</span>
          </div>
          {users.map((u) => (
            <div className="db-row" key={u.id}>
              <span className="db-name">
                {u.username}
                <span className="db-pw" style={{ display: 'block' }}>
                  {u.email}
                </span>
              </span>
              <span>
                <div
                  className={`toggle-sw ${u.is_admin ? 'on' : ''}`}
                  onClick={() => saving === null && handleToggleAdmin(u)}
                >
                  <div className="toggle-knob" />
                </div>
              </span>
              <span>
                <div
                  className={`toggle-sw ${u.is_active ? 'on' : ''}`}
                  onClick={() => saving === null && handleToggleActive(u)}
                >
                  <div className="toggle-knob" />
                </div>
              </span>
              <span style={{ display: 'flex', gap: 6 }}>
                <input
                  type="number"
                  placeholder="unlimited"
                  value={drafts[u.id]?.serverLimit ?? ''}
                  onChange={(e) =>
                    setDrafts((d) => ({ ...d, [u.id]: { serverLimit: e.target.value } }))
                  }
                  style={{
                    width: 90,
                    background: 'rgba(255,255,255,.04)',
                    border: '1px solid var(--border)',
                    borderRadius: 8,
                    padding: '9px 12px',
                    color: 'var(--text)',
                    fontFamily: 'inherit',
                    fontSize: 12.5,
                    outline: 'none',
                  }}
                />
                <button className="btn-sm" disabled={saving === u.id} onClick={() => handleSaveLimit(u)}>
                  Save
                </button>
              </span>
            </div>
          ))}
          {users.length === 0 && <p className="srv-desc" style={{ padding: 16 }}>No users yet.</p>}
        </div>
      )}
    </div>
  );
}
