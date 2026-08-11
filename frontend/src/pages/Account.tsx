import { useEffect, useState } from 'react';
import QRCode from 'qrcode';
import { api } from '../api/client';
import { t } from '../i18n';
import type { ApiKey, CreateApiKeyResponse, SSHKey, TwoFASetup, TwoFAStatus } from '../types';
import { API_KEY_PERMISSIONS } from '../types';
import { canGenerateKeyPair, downloadPrivateKey, generateEd25519KeyPair } from '../sshKeygen';

function loadUsername(): string {
  try {
    const raw = localStorage.getItem('user');
    if (!raw) return 'yourusername';
    return (JSON.parse(raw) as { username: string }).username;
  } catch {
    return 'yourusername';
  }
}

export function Account() {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [name, setName] = useState('');
  const [keyPermissions, setKeyPermissions] = useState<string[]>([]);
  const [justCreated, setJustCreated] = useState<CreateApiKeyResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [sshKeys, setSSHKeys] = useState<SSHKey[]>([]);
  const [sshKeyName, setSSHKeyName] = useState('');
  const [sshPublicKey, setSSHPublicKey] = useState('');
  const [sshError, setSSHError] = useState<string | null>(null);
  const [sshSubmitting, setSSHSubmitting] = useState(false);
  const [generatingKey, setGeneratingKey] = useState(false);
  const [justGenerated, setJustGenerated] = useState(false);

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [passwordSuccess, setPasswordSuccess] = useState(false);
  const [changingPassword, setChangingPassword] = useState(false);

  const [twofaStatus, setTwofaStatus] = useState<TwoFAStatus | null>(null);
  const [twofaSetup, setTwofaSetup] = useState<TwoFASetup | null>(null);
  const [qrCodeUrl, setQrCodeUrl] = useState<string | null>(null);
  const [verifyCode, setVerifyCode] = useState('');
  const [disablePassword, setDisablePassword] = useState('');
  const [twofaError, setTwofaError] = useState<string | null>(null);
  const [twofaBusy, setTwofaBusy] = useState(false);

  function refresh() {
    api.listApiKeys().then(setKeys).catch(() => {});
  }

  function refreshSSHKeys() {
    api.listSSHKeys().then(setSSHKeys).catch(() => {});
  }

  function refreshTwofa() {
    api.get2FAStatus().then(setTwofaStatus).catch(() => {});
  }

  useEffect(refresh, []);
  useEffect(refreshSSHKeys, []);
  useEffect(refreshTwofa, []);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const created = await api.createApiKey(name, keyPermissions);
      setJustCreated(created);
      setName('');
      setKeyPermissions([]);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  function toggleKeyPermission(code: string) {
    setKeyPermissions((list) =>
      list.includes(code) ? list.filter((p) => p !== code) : [...list, code],
    );
  }

  async function handleDelete(id: number) {
    try {
      await api.deleteApiKey(id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleCreateSSHKey(e: React.FormEvent) {
    e.preventDefault();
    setSSHSubmitting(true);
    setSSHError(null);
    try {
      await api.createSSHKey(sshKeyName.trim() || 'Uploaded key', sshPublicKey);
      setSSHKeyName('');
      setSSHPublicKey('');
      refreshSSHKeys();
    } catch (err) {
      setSSHError(err instanceof Error ? err.message : String(err));
    } finally {
      setSSHSubmitting(false);
    }
  }

  async function handleGenerateKeyPair() {
    setSSHError(null);
    setJustGenerated(false);
    setGeneratingKey(true);
    try {
      const name = sshKeyName.trim() || 'Generated key';
      const { publicKeyLine, privateKeyPem } = await generateEd25519KeyPair(`${loadUsername()}@powernode`);
      await api.createSSHKey(name, publicKeyLine);
      downloadPrivateKey(privateKeyPem, `${name.replace(/[^a-z0-9_-]+/gi, '_')}.pem`);
      setSSHKeyName('');
      setJustGenerated(true);
      refreshSSHKeys();
    } catch (err) {
      setSSHError(err instanceof Error ? err.message : String(err));
    } finally {
      setGeneratingKey(false);
    }
  }

  async function handleDeleteSSHKey(id: number) {
    try {
      await api.deleteSSHKey(id);
      refreshSSHKeys();
    } catch (err) {
      setSSHError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleChangePassword(e: React.FormEvent) {
    e.preventDefault();
    setChangingPassword(true);
    setPasswordError(null);
    setPasswordSuccess(false);
    try {
      await api.changePassword(currentPassword, newPassword);
      setCurrentPassword('');
      setNewPassword('');
      setPasswordSuccess(true);
    } catch (err) {
      setPasswordError(err instanceof Error ? err.message : String(err));
    } finally {
      setChangingPassword(false);
    }
  }

  async function handleStartSetup() {
    setTwofaError(null);
    try {
      const setup = await api.setup2FA();
      setTwofaSetup(setup);
      setQrCodeUrl(await QRCode.toDataURL(setup.otpauth_url, { width: 220, margin: 1 }));
    } catch (err) {
      setTwofaError(err instanceof Error ? err.message : String(err));
    }
  }

  function handleCancelSetup() {
    setTwofaSetup(null);
    setQrCodeUrl(null);
    setVerifyCode('');
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    setTwofaBusy(true);
    setTwofaError(null);
    try {
      await api.verify2FA(verifyCode);
      handleCancelSetup();
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
        <h1>{t('account.title')}</h1>
        <p>{t('account.subtitle')}</p>
      </div>

      <div className="acc-grid">
        <div className="acc-card">
          <div className="acc-card-title">{t('account.apiKeys')}</div>

          {justCreated && (
            <div className="api-item" style={{ marginBottom: 12 }}>
              <span className="api-key">{justCreated.token}</span>
              <button
                className="btn-sm"
                onClick={() => navigator.clipboard?.writeText(justCreated.token)}
              >
                {t('common.copy')}
              </button>
            </div>
          )}

          <div className="api-list">
            {keys.map((k) => (
              <div className="api-item" key={k.id} style={{ flexDirection: 'column', alignItems: 'stretch', gap: 6 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <span className="api-memo">{k.name}</span>
                  <span className="api-used">
                    {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : t('account.neverUsed')}
                  </span>
                  <button className="file-act-btn del" onClick={() => handleDelete(k.id)}>
                    {t('common.delete')}
                  </button>
                </div>
                <span className="srv-desc" style={{ fontSize: 11 }}>
                  {k.permissions.length === 0
                    ? t('account.fullAccess')
                    : k.permissions.join(', ')}
                </span>
              </div>
            ))}
            {keys.length === 0 && <p className="srv-desc">{t('account.noApiKeysYet')}</p>}
          </div>

          <form onSubmit={handleCreate} style={{ marginTop: 16 }}>
            <div className="sfield" style={{ marginBottom: 14 }}>
              <label htmlFor="key-name">{t('account.newKeyName')}</label>
              <input
                id="key-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('account.newKeyPlaceholder')}
                required
              />
            </div>
            <p className="srv-desc" style={{ marginBottom: 8 }}>
              {t('account.permissionsHint')}
            </p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, marginBottom: 14 }}>
              {API_KEY_PERMISSIONS.map((p) => (
                <label
                  key={p.code}
                  style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12 }}
                >
                  <div
                    className={`toggle-sw ${keyPermissions.includes(p.code) ? 'on' : ''}`}
                    onClick={() => toggleKeyPermission(p.code)}
                  >
                    <div className="toggle-knob" />
                  </div>
                  {p.label}
                </label>
              ))}
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
                {submitting ? t('common.saving') : t('account.createKey')}
              </button>
            </div>
          </form>
        </div>

        <div className="acc-card">
          <div className="acc-card-title">{t('account.sshKeysTitle')}</div>
          <p className="srv-desc" style={{ marginBottom: 12 }}>
            {t('account.sshKeysHintPrefix')}
            <code>{loadUsername()}.&lt;server-id&gt;</code>
            {t('account.sshKeysHintMiddle')}
            <code>2022</code> {t('account.sshKeysHintSuffix')}
          </p>

          <div className="api-list">
            {sshKeys.map((k) => (
              <div className="api-item" key={k.id} style={{ flexDirection: 'column', alignItems: 'stretch', gap: 6 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <span className="api-memo">{k.name}</span>
                  <button className="file-act-btn del" onClick={() => handleDeleteSSHKey(k.id)}>
                    {t('common.delete')}
                  </button>
                </div>
                <span className="srv-desc" style={{ fontSize: 11, fontFamily: 'var(--font-mono)' }}>
                  {k.fingerprint}
                </span>
              </div>
            ))}
            {sshKeys.length === 0 && <p className="srv-desc">{t('account.noSshKeysYet')}</p>}
          </div>

          {justGenerated && (
            <div className="login-error show" style={{ marginTop: 16, borderColor: 'var(--green)', color: 'var(--green)' }}>
              {t('account.keyPairGenerated')}
            </div>
          )}

          <div className="sfield" style={{ marginTop: 16, marginBottom: 0 }}>
            <label htmlFor="ssh-key-name">{t('account.keyName')}</label>
            <input
              id="ssh-key-name"
              value={sshKeyName}
              onChange={(e) => setSSHKeyName(e.target.value)}
              placeholder={t('account.keyNamePlaceholder')}
            />
          </div>

          {canGenerateKeyPair() && (
            <div className="settings-foot" style={{ marginTop: 10 }}>
              <button
                type="button"
                className="btn-primary"
                onClick={handleGenerateKeyPair}
                disabled={generatingKey}
                style={{ width: 'auto', padding: '10px 20px' }}
              >
                {generatingKey ? t('nodes.generating') : t('account.generateKeyPair')}
              </button>
            </div>
          )}

          <p className="srv-desc" style={{ marginTop: canGenerateKeyPair() ? 16 : 0, marginBottom: 8 }}>
            {canGenerateKeyPair() ? t('account.orPasteExisting') : t('account.pasteExisting')}
          </p>

          <form onSubmit={handleCreateSSHKey}>
            <div className="sfield" style={{ marginBottom: 14 }}>
              <label htmlFor="ssh-key-value">{t('account.publicKey')}</label>
              <textarea
                id="ssh-key-value"
                value={sshPublicKey}
                onChange={(e) => setSSHPublicKey(e.target.value)}
                placeholder={t('account.publicKeyPlaceholder')}
                rows={3}
                required
                style={{
                  background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 9,
                  padding: '11px 14px', color: 'var(--text)', fontFamily: 'var(--font-mono)',
                  fontSize: 12, width: '100%', resize: 'vertical',
                }}
              />
            </div>
            {sshError && (
              <div className="login-error show" style={{ marginTop: 12, marginBottom: 12 }}>
                {sshError}
              </div>
            )}
            <div className="settings-foot">
              <button
                className="btn-primary"
                type="submit"
                disabled={sshSubmitting}
                style={{ width: 'auto', padding: '10px 20px' }}
              >
                {sshSubmitting ? t('nodes.adding') : t('account.addKey')}
              </button>
            </div>
          </form>
        </div>

        <div className="acc-card">
          <div className="acc-card-title">{t('account.changePassword')}</div>
          <form onSubmit={handleChangePassword}>
            <div className="sfield" style={{ marginBottom: 14 }}>
              <label htmlFor="current-password">{t('account.currentPassword')}</label>
              <input
                id="current-password"
                type="password"
                autoComplete="current-password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                required
              />
            </div>
            <div className="sfield" style={{ marginBottom: 14 }}>
              <label htmlFor="new-password">{t('account.newPassword')}</label>
              <input
                id="new-password"
                type="password"
                autoComplete="new-password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder={t('users.passwordPlaceholder')}
                required
              />
            </div>
            {passwordError && (
              <div className="login-error show" style={{ marginBottom: 12 }}>
                {passwordError}
              </div>
            )}
            {passwordSuccess && (
              <p className="srv-desc" style={{ color: 'var(--green)', marginBottom: 12 }}>
                {t('account.passwordUpdated')}
              </p>
            )}
            <div className="settings-foot">
              <button
                className="btn-primary"
                type="submit"
                disabled={changingPassword}
                style={{ width: 'auto', padding: '10px 20px' }}
              >
                {changingPassword ? t('account.updating') : t('account.updatePassword')}
              </button>
            </div>
          </form>
        </div>

        <div className="acc-card">
          <div className="acc-card-title">{t('account.twofaTitle')}</div>

          <div style={{ textAlign: 'center' }}>
            <div className="twofa-icon">🔒</div>
            <div className="twofa-title">
              {twofaStatus === null ? t('common.loading') : twofaStatus.enabled ? t('account.twofaEnabled') : t('account.twofaDisabled')}
            </div>
            <div className="twofa-desc">
              {twofaStatus?.enabled
                ? t('account.twofaEnabledHint')
                : t('account.twofaDisabledHint')}
            </div>
            {twofaStatus?.enabled && <div className="twofa-status">{t('account.twofaActive')}</div>}
          </div>

          {twofaError && (
            <div className="login-error show" style={{ marginBottom: 12 }}>
              {twofaError}
            </div>
          )}

          {twofaStatus && !twofaStatus.enabled && !twofaSetup && (
            <button className="btn-primary" onClick={handleStartSetup}>
              {t('account.enable2fa')}
            </button>
          )}

          {twofaSetup && (
            <div>
              <p className="srv-desc" style={{ marginBottom: 10 }}>
                {t('account.scanHint')}
              </p>
              {qrCodeUrl && (
                <div style={{ textAlign: 'center', marginBottom: 16 }}>
                  <img
                    src={qrCodeUrl}
                    alt="2FA setup QR code"
                    width={220}
                    height={220}
                    style={{ borderRadius: 10 }}
                  />
                </div>
              )}
              <div className="api-item" style={{ marginBottom: 8 }}>
                <span className="api-key">{twofaSetup.otpauth_url}</span>
                <button
                  className="btn-sm"
                  onClick={() => navigator.clipboard?.writeText(twofaSetup.otpauth_url)}
                >
                  {t('common.copy')}
                </button>
              </div>
              <div className="api-item" style={{ marginBottom: 16 }}>
                <span className="api-key">{twofaSetup.secret}</span>
                <button
                  className="btn-sm"
                  onClick={() => navigator.clipboard?.writeText(twofaSetup.secret)}
                >
                  {t('common.copy')}
                </button>
              </div>
              <form onSubmit={handleVerify}>
                <div className="sfield">
                  <label htmlFor="verify-code">{t('account.enterCodeToConfirm')}</label>
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
                    {twofaBusy ? t('login.verifying') : t('account.verifyAndEnable')}
                  </button>
                  <button className="btn-sm" type="button" onClick={handleCancelSetup}>
                    {t('common.cancel')}
                  </button>
                </div>
              </form>
            </div>
          )}

          {twofaStatus?.enabled && (
            <form onSubmit={handleDisable}>
              <div className="sfield">
                <label htmlFor="disable-password">{t('account.disablePassword')}</label>
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
                  {twofaBusy ? t('account.disabling') : t('account.disable2fa')}
                </button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
