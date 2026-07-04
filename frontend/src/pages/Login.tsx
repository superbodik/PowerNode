import { useState } from 'react';
import { api } from '../api/client';

interface Props {
  onLoggedIn: () => void;
}

export function Login({ onLoggedIn }: Props) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const { access_token, user } = await api.login(email, password);
      localStorage.setItem('access_token', access_token);
      localStorage.setItem('user', JSON.stringify(user));
      onLoggedIn();
    } catch {
      setError('Invalid email or password');
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
            <div className="title">Panel</div>
            <div className="sub">Control</div>
          </div>
        </div>

        <div className="login-head">
          <h1>Sign in</h1>
          <p>Use your panel account to continue.</p>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="form-field">
            <label htmlFor="email">Email</label>
            <input
              id="email"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>
          <div className="form-field">
            <label htmlFor="password">Password</label>
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
            {submitting ? 'Signing in…' : 'Sign in'}
          </button>

          {error && <div className="login-error show">{error}</div>}
        </form>
      </div>
    </div>
  );
}
