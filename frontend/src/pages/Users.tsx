import { useEffect, useMemo, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import type { PanelUser } from '../types';

export function Users() {
  const [users, setUsers] = useState<PanelUser[] | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<number, { serverLimit: string }>>({});
  const [saving, setSaving] = useState<number | null>(null);
  const [query, setQuery] = useState('');

  const [createForm, setCreateForm] = useState({
    email: '',
    username: '',
    password: '',
    is_admin: false,
    server_limit: '',
  });
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const [expandedUserId, setExpandedUserId] = useState<number | null>(null);
  const [resetPasswordValue, setResetPasswordValue] = useState('');
  const [resetSubmitting, setResetSubmitting] = useState(false);
  const [resetSuccess, setResetSuccess] = useState(false);

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

  const filteredUsers = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q || !users) return users ?? [];
    return users.filter(
      (u) => u.username.toLowerCase().includes(q) || u.email.toLowerCase().includes(q),
    );
  }, [users, query]);

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

  function toggleExpand(userId: number) {
    setExpandedUserId((current) => (current === userId ? null : userId));
    setResetPasswordValue('');
    setResetSuccess(false);
  }

  async function handleResetPassword(e: React.FormEvent, userId: number) {
    e.preventDefault();
    setResetSubmitting(true);
    setResetSuccess(false);
    setError(null);
    try {
      await api.resetUserPassword(userId, resetPasswordValue);
      setResetPasswordValue('');
      setResetSuccess(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setResetSubmitting(false);
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setCreating(true);
    setCreateError(null);
    try {
      await api.createUser({
        email: createForm.email,
        username: createForm.username,
        password: createForm.password,
        is_admin: createForm.is_admin,
        server_limit: createForm.server_limit.trim() === '' ? null : Number(createForm.server_limit),
      });
      setCreateForm({ email: '', username: '', password: '', is_admin: false, server_limit: '' });
      refresh();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : String(err));
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>{t('users.title')}</h1>
        <p>{t('users.subtitle')}</p>
      </div>

      {!forbidden && (
        <div className="settings-card" style={{ marginBottom: 24 }}>
          <div className="settings-card-title">{t('users.createUser')}</div>
          <form onSubmit={handleCreate}>
            <div className="settings-grid">
              <div className="sfield">
                <label htmlFor="user-email">{t('users.email')}</label>
                <input
                  id="user-email"
                  type="email"
                  value={createForm.email}
                  onChange={(e) => setCreateForm((f) => ({ ...f, email: e.target.value }))}
                  required
                />
              </div>
              <div className="sfield">
                <label htmlFor="user-username">{t('users.username')}</label>
                <input
                  id="user-username"
                  value={createForm.username}
                  onChange={(e) => setCreateForm((f) => ({ ...f, username: e.target.value }))}
                  required
                />
              </div>
              <div className="sfield">
                <label htmlFor="user-password">{t('users.password')}</label>
                <input
                  id="user-password"
                  type="password"
                  autoComplete="new-password"
                  value={createForm.password}
                  onChange={(e) => setCreateForm((f) => ({ ...f, password: e.target.value }))}
                  placeholder={t('users.passwordPlaceholder')}
                  required
                />
              </div>
              <div className="sfield">
                <label htmlFor="user-limit">{t('users.serverLimit')}</label>
                <input
                  id="user-limit"
                  type="number"
                  value={createForm.server_limit}
                  onChange={(e) => setCreateForm((f) => ({ ...f, server_limit: e.target.value }))}
                  placeholder={t('users.unlimited')}
                />
              </div>
            </div>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, marginBottom: 14 }}>
              <div
                className={`toggle-sw ${createForm.is_admin ? 'on' : ''}`}
                onClick={() => setCreateForm((f) => ({ ...f, is_admin: !f.is_admin }))}
              >
                <div className="toggle-knob" />
              </div>
              {t('users.admin')}
            </label>
            {createError && <div className="login-error show" style={{ marginBottom: 12 }}>{createError}</div>}
            <div className="settings-foot">
              <button className="btn-primary" type="submit" disabled={creating} style={{ width: 'auto', padding: '10px 20px' }}>
                {creating ? t('users.creatingUser') : t('users.createUser')}
              </button>
            </div>
          </form>
        </div>
      )}

      {forbidden && <p className="srv-desc">{t('users.forbidden')}</p>}

      {error && <div className="login-error show" style={{ marginBottom: 16 }}>{error}</div>}

      {!forbidden && users === null && <p className="srv-desc">{t('common.loading')}</p>}

      {users && (
        <>
        <div className="dash-toolbar">
          <div className="search-wrap">
            <span className="search-icon">⌕</span>
            <input
              type="text"
              placeholder={t('users.searchPlaceholder')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
        </div>
        <div className="db-table">
          <div className="db-head">
            <span>{t('users.colUser')}</span>
            <span>{t('users.colAdmin')}</span>
            <span>{t('users.colActive')}</span>
            <span>{t('users.colServerLimit')}</span>
          </div>
          {filteredUsers.map((u) => (
            <div key={u.id}>
            <div className="db-row">
              <span
                className="db-name"
                style={{ cursor: 'pointer' }}
                onClick={() => toggleExpand(u.id)}
              >
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
                  placeholder={t('users.unlimited')}
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
                  {t('common.save')}
                </button>
              </span>
            </div>
            {expandedUserId === u.id && (
              <div style={{ padding: '14px 18px', borderBottom: '1px solid rgba(192,100,120,.06)' }}>
                <form
                  onSubmit={(e) => handleResetPassword(e, u.id)}
                  style={{ display: 'flex', gap: 8, alignItems: 'flex-end', flexWrap: 'wrap' }}
                >
                  <div className="sfield" style={{ margin: 0 }}>
                    <label htmlFor={`reset-pw-${u.id}`}>{t('users.resetPasswordFor', { username: u.username })}</label>
                    <input
                      id={`reset-pw-${u.id}`}
                      type="password"
                      autoComplete="new-password"
                      value={resetPasswordValue}
                      onChange={(e) => setResetPasswordValue(e.target.value)}
                      placeholder={t('users.passwordPlaceholder')}
                      required
                    />
                  </div>
                  <button className="btn-primary" type="submit" disabled={resetSubmitting} style={{ width: 'auto', padding: '10px 20px' }}>
                    {resetSubmitting ? t('users.resetting') : t('users.resetPassword')}
                  </button>
                  {resetSuccess && (
                    <span className="srv-desc" style={{ color: 'var(--green)' }}>
                      {t('users.passwordResetDone')}
                    </span>
                  )}
                </form>
              </div>
            )}
            </div>
          ))}
          {users.length === 0 && <p className="srv-desc" style={{ padding: 16 }}>{t('users.noUsersYet')}</p>}
          {users.length > 0 && filteredUsers.length === 0 && (
            <p className="srv-desc" style={{ padding: 16 }}>{t('users.noUsersMatch')}</p>
          )}
        </div>
        </>
      )}
    </div>
  );
}
