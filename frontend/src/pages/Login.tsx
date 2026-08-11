import { useState } from 'react';
import { api, storeTokens, TOTPRequiredError } from '../api/client';
import { t } from '../i18n';

interface Props {
  onLoggedIn: () => void;
}

export function Login({ onLoggedIn }: Props) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [totpCode, setTotpCode] = useState('');
  const [needsTotp, setNeedsTotp] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const tokens = await api.login(email, password, needsTotp ? totpCode : undefined);
      storeTokens(tokens);
      onLoggedIn();
    } catch (err) {
      if (err instanceof TOTPRequiredError) {
        setNeedsTotp(true);
      } else {
        setError(needsTotp ? t('login.invalidWithCode') : t('login.invalid'));
      }
    } finally {
      setSubmitting(false);
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

        {!needsTotp ? (
          <div className="login-step" key="credentials">
            <div className="login-head">
              <h1>{t('login.signIn')}</h1>
              <p>{t('login.signInHint')}</p>
            </div>

            <form onSubmit={handleSubmit}>
              <div className="form-field">
                <label htmlFor="email">{t('login.email')}</label>
                <input
                  id="email"
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  autoFocus
                  required
                />
              </div>
              <div className="form-field">
                <label htmlFor="password">{t('login.password')}</label>
                <input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </div>

              <button className="btn-primary" type="submit" disabled={submitting}>
                {submitting ? t('login.signingIn') : t('login.signIn')}
              </button>

              {error && <div className="login-error show">{error}</div>}
            </form>
          </div>
        ) : (
          <div className="login-step" key="totp">
            <div className="login-head">
              <h1>{t('login.verificationCode')}</h1>
              <p>{t('login.verificationHint')}</p>
            </div>

            <form onSubmit={handleSubmit}>
              <div className="form-field">
                <label htmlFor="totp-code">{t('login.authenticatorCode')}</label>
                <input
                  id="totp-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  placeholder="123456"
                  autoFocus
                  required
                />
              </div>

              <button className="btn-primary" type="submit" disabled={submitting}>
                {submitting ? t('login.verifying') : t('login.verifyAndSignIn')}
              </button>

              {error && <div className="login-error show">{error}</div>}

              <button
                type="button"
                className="login-back"
                onClick={() => {
                  setNeedsTotp(false);
                  setTotpCode('');
                  setError(null);
                }}
              >
                {t('login.back')}
              </button>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}
