import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ApiKey, CreateApiKeyResponse, TwoFASetup, TwoFAStatus } from '../types';

export function Account() {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [name, setName] = useState('');
  const [justCreated, setJustCreated] = useState<CreateApiKeyResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [twofaStatus, setTwofaStatus] = useState<TwoFAStatus | null>(null);
  const [twofaSetup, setTwofaSetup] = useState<TwoFASetup | null>(null);
  const [verifyCode, setVerifyCode] = useState('');
  const [disablePassword, setDisablePassword] = useState('');
  const [twofaError, setTwofaError] = useState<string | null>(null);
  const [twofaBusy, setTwofaBusy] = useState(false);

  function refresh() {
    api.listApiKeys().then(setKeys).catch(() => {});
  }

  function refreshTwofa() {
    api.get2FAStatus().then(setTwofaStatus).catch(() => {});
  }

  useEffect(refresh, []);
  useEffect(refreshTwofa, []);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const created = await api.createApiKey(name);
      setJustCreated(created);
      setName('');
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(id: number) {
    try {
      await api.deleteApiKey(id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleStartSetup() {
    setTwofaError(null);
    try {
      setTwofaSetup(await api.setup2FA());
    } catch (err) {
      setTwofaError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    setTwofaBusy(true);
    setTwofaError(null);
    try {
      await api.verify2FA(verifyCode);
      setTwofaSetup(null);
      setVerifyCode('');
      refreshTwofa();
    } catch (err) {
      setTwofaError(err instanceof Error ? err.message : String(err));
    } finally {
      setTwofaBusy(false);
    }
  }

  async function handleDisable(e: React.FormEvent) {
    e.preventDefault();
    setTwofaBusy(true);
    setTwofaError(null);
    try {
      await api.disable2FA(disablePassword);
      setDisablePassword('');
      refreshTwofa();
    } catch (err) {
      setTwofaError(err instanceof Error ? err.message : String(err));
    } finally {
      setTwofaBusy(false);
    }
  }

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>Account</h1>
        <p>API keys for programmatic access.</p>
      </div>

      <div className="acc-grid">
        <div className="acc-card">
          <div className="acc-card-title">API Keys</div>

          {justCreated && (
            <div className="api-item" style={{ marginBottom: 12 }}>
              <span className="api-key">{justCreated.token}</span>
              <button
                className="btn-sm"
                onClick={() => navigator.clipboard?.writeText(justCreated.token)}
              >
                Copy
              </button>
            </div>
          )}

          <div className="api-list">
            {keys.map((k) => (
              <div className="api-item" key={k.id}>
                <span className="api-memo">{k.name}</span>
                <span className="api-used">
                  {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : 'never used'}
                </span>
                <button className="file-act-btn del" onClick={() => handleDelete(k.id)}>
                  Delete
                </button>
              </div>
            ))}
            {keys.length === 0 && <p className="srv-desc">No API keys yet.</p>}
          </div>

          <form onSubmit={handleCreate} style={{ marginTop: 16 }}>
            <div className="sfield">
              <label htmlFor="key-name">New key name</label>
              <input
                id="key-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. CI deploy"
                required
              />
            </div>
            {error && (
              <div className="login-error show" style={{ marginTop: 12 }}>
                {error}
              </div>
            )}
            <div className="settings-foot">
              <button
                className="btn-primary"
                type="submit"
                disabled={submitting}
                style={{ width: 'auto', padding: '10px 20px' }}
              >
                {submitting ? 'Creating…' : 'Create key'}
              </button>
            </div>
          </form>
        </div>

        <div className="acc-card">
          <div className="acc-card-title">Two-Factor Authentication</div>

          <div style={{ textAlign: 'center' }}>
            <div className="twofa-icon">🔒</div>
            <div className="twofa-title">
              {twofaStatus === null ? 'Loading…' : twofaStatus.enabled ? 'Enabled' : 'Disabled'}
            </div>
            <div className="twofa-desc">
              {twofaStatus?.enabled
                ? 'Your account requires a code from your authenticator app at sign-in.'
                : 'Add an authenticator app (Google Authenticator, Authy, etc.) for a second layer of protection at sign-in.'}
            </div>
            {twofaStatus?.enabled && <div className="twofa-status">✓ Active</div>}
          </div>

          {twofaError && (
            <div className="login-error show" style={{ marginBottom: 12 }}>
              {twofaError}
            </div>
          )}

          {twofaStatus && !twofaStatus.enabled && !twofaSetup && (
            <button className="btn-primary" onClick={handleStartSetup}>
              Enable 2FA
            </button>
          )}

          {twofaSetup && (
            <div>
              <p className="srv-desc" style={{ marginBottom: 10 }}>
                Scan this with your authenticator app, or add it manually with the secret below.
              </p>
              <div className="api-item" style={{ marginBottom: 8 }}>
                <span className="api-key">{twofaSetup.otpauth_url}</span>
                <button
                  className="btn-sm"
                  onClick={() => navigator.clipboard?.writeText(twofaSetup.otpauth_url)}
                >
                  Copy
                </button>
              </div>
              <div className="api-item" style={{ marginBottom: 16 }}>
                <span className="api-key">{twofaSetup.secret}</span>
                <button
                  className="btn-sm"
                  onClick={() => navigator.clipboard?.writeText(twofaSetup.secret)}
                >
                  Copy
                </button>
              </div>
              <form onSubmit={handleVerify}>
                <div className="sfield">
                  <label htmlFor="verify-code">Enter the 6-digit code to confirm</label>
                  <input
                    id="verify-code"
                    inputMode="numeric"
                    value={verifyCode}
                    onChange={(e) => setVerifyCode(e.target.value)}
                    placeholder="123456"
                    required
                  />
                </div>
                <div className="settings-foot" style={{ display: 'flex', gap: 8 }}>
                  <button
                    className="btn-primary"
                    type="submit"
                    disabled={twofaBusy}
                    style={{ width: 'auto', padding: '10px 20px' }}
                  >
                    {twofaBusy ? 'Verifying…' : 'Verify & enable'}
                  </button>
                  <button className="btn-sm" type="button" onClick={() => setTwofaSetup(null)}>
                    Cancel
                  </button>
                </div>
              </form>
            </div>
          )}

          {twofaStatus?.enabled && (
            <form onSubmit={handleDisable}>
              <div className="sfield">
                <label htmlFor="disable-password">Password (to disable)</label>
                <input
                  id="disable-password"
                  type="password"
                  autoComplete="current-password"
                  value={disablePassword}
                  onChange={(e) => setDisablePassword(e.target.value)}
                  required
                />
              </div>
              <div className="settings-foot">
                <button
                  className="btn-danger"
                  type="submit"
                  disabled={twofaBusy}
                  style={{ width: 'auto', padding: '10px 20px' }}
                >
                  {twofaBusy ? 'Disabling…' : 'Disable 2FA'}
                </button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
