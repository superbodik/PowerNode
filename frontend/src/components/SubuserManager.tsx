import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import type { Subuser } from '../types';
import { SUBUSER_PERMISSIONS } from '../types';

interface Props {
  uuid: string;
}

export function SubuserManager({ uuid }: Props) {
  const [subusers, setSubusers] = useState<Subuser[] | null>(null);
  const [forbidden, setForbidden] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [email, setEmail] = useState('');
  const [newPermissions, setNewPermissions] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);

  function refresh() {
    api
      .listSubusers(uuid)
      .then((s) => {
        setSubusers(s);
        setForbidden(false);
      })
      .catch(() => {
        setSubusers(null);
        setForbidden(true);
      });
  }

  useEffect(refresh, [uuid]);

  function togglePermission(list: string[], code: string): string[] {
    return list.includes(code) ? list.filter((p) => p !== code) : [...list, code];
  }

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.addSubuser(uuid, email, newPermissions);
      setEmail('');
      setNewPermissions([]);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleTogglePermission(s: Subuser, code: string) {
    const updated = togglePermission(s.permissions, code);
    try {
      await api.updateSubuser(uuid, s.id, updated);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleRemove(s: Subuser) {
    if (!window.confirm(t('subusers.confirmRemove', { email: s.email }))) return;
    try {
      await api.removeSubuser(uuid, s.id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  if (forbidden) {
    return (
      <p className="srv-desc">
        {t('subusers.forbidden')}
      </p>
    );
  }

  if (subusers === null) {
    return <p className="srv-desc">{t('common.loading')}</p>;
  }

  return (
    <div>
      {error && (
        <div className="login-error show" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      <div className="settings-card" style={{ marginBottom: 20 }}>
        <div className="settings-card-title">{t('subusers.addCollaborator')}</div>
        <form onSubmit={handleAdd}>
          <div className="sfield" style={{ marginBottom: 14 }}>
            <label htmlFor="subuser-email">{t('subusers.userEmail')}</label>
            <input
              id="subuser-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={t('subusers.emailPlaceholder')}
              required
            />
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 16, marginBottom: 16 }}>
            {SUBUSER_PERMISSIONS.map((p) => (
              <label
                key={p.code}
                style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12.5 }}
              >
                <div
                  className={`toggle-sw ${newPermissions.includes(p.code) ? 'on' : ''}`}
                  onClick={() => setNewPermissions((list) => togglePermission(list, p.code))}
                >
                  <div className="toggle-knob" />
                </div>
                {p.label}
              </label>
            ))}
          </div>
          <div className="settings-foot">
            <button
              className="btn-primary"
              type="submit"
              disabled={submitting}
              style={{ width: 'auto', padding: '10px 20px' }}
            >
              {submitting ? t('subusers.adding') : t('subusers.addCollaboratorButton')}
            </button>
          </div>
        </form>
      </div>

      <div className="sch-list">
        {subusers.map((s) => (
          <div className="sch-card" key={s.id}>
            <div className="sch-head">
              <span className="sch-name">{s.email}</span>
              <button className="file-act-btn del" onClick={() => handleRemove(s)}>
                {t('subusers.remove')}
              </button>
            </div>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 16 }}>
              {SUBUSER_PERMISSIONS.map((p) => (
                <label
                  key={p.code}
                  style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12.5 }}
                >
                  <div
                    className={`toggle-sw ${s.permissions.includes(p.code) ? 'on' : ''}`}
                    onClick={() => handleTogglePermission(s, p.code)}
                  >
                    <div className="toggle-knob" />
                  </div>
                  {p.label}
                </label>
              ))}
            </div>
          </div>
        ))}
        {subusers.length === 0 && <p className="srv-desc">{t('subusers.noCollaboratorsYet')}</p>}
      </div>
    </div>
  );
}
