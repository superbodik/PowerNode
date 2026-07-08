import { useState } from 'react';
import QRCode from 'qrcode';
import { api } from '../api/client';
import type { TwoFASetup } from '../types';

interface Props {
  onVerified: () => void;
  onLogout: () => void;
}

export function TwoFactorGate({ onVerified, onLogout }: Props) {
  const [setup, setSetup] = useState<TwoFASetup | null>(null);
  const [qrCodeUrl, setQrCodeUrl] = useState<string | null>(null);
  const [verifyCode, setVerifyCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleStartSetup() {
    setError(null);
    try {
      const result = await api.setup2FA();
      setSetup(result);
      setQrCodeUrl(await QRCode.toDataURL(result.otpauth_url, { width: 220, margin: 1 }));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.verify2FA(verifyCode);
      onVerified();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div id="page-login" className="page active">
      <div className="ambient">
        <div className="blob b1" />
        <div className="blob b2" />
      </div>

      <div className="login-box">
        <div className="login-logo">
          <div className="login-logo-text">
            <div className="title">Power</div>
            <div className="sub">Node</div>
          </div>
        </div>

        <div className="login-head">
          <h1>2FA required</h1>
          <p>Admin accounts on this panel must have two-factor authentication enabled before continuing.</p>
        </div>

        {error && <div className="login-error show">{error}</div>}

        {!setup ? (
          <button className="btn-primary" style={{ width: '100%' }} onClick={handleStartSetup}>
            Set up 2FA
          </button>
        ) : (
          <div>
            <p className="srv-desc" style={{ marginBottom: 10 }}>
              Scan this with your authenticator app, or add it manually with the secret below.
            </p>
            {qrCodeUrl && (
              <div style={{ textAlign: 'center', marginBottom: 16 }}>
                <img src={qrCodeUrl} alt="2FA setup QR code" width={220} height={220} style={{ borderRadius: 10 }} />
              </div>
            )}
            <div className="api-item" style={{ marginBottom: 16 }}>
              <span className="api-key">{setup.secret}</span>
              <button className="btn-sm" onClick={() => navigator.clipboard?.writeText(setup.secret)}>
                Copy
              </button>
            </div>
            <form onSubmit={handleVerify}>
              <div className="sfield">
                <label htmlFor="gate-verify-code">Enter the 6-digit code to confirm</label>
                <input
                  id="gate-verify-code"
                  inputMode="numeric"
                  value={verifyCode}
                  onChange={(e) => setVerifyCode(e.target.value)}
                  placeholder="123456"
                  required
                />
              </div>
              <button
                className="btn-primary"
                type="submit"
                disabled={busy}
                style={{ width: '100%', marginTop: 12 }}
              >
                {busy ? 'Verifying…' : 'Verify & continue'}
              </button>
            </form>
          </div>
        )}

        <div style={{ textAlign: 'center', marginTop: 20 }}>
          <span className="bc-sep" style={{ cursor: 'pointer' }} onClick={onLogout}>
            Log out
          </span>
        </div>
      </div>
    </div>
  );
}
