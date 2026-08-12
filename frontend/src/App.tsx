import { useEffect, useState } from 'react';
import { api, clearTokens } from './api/client';
import { locale, setLocale, t } from './i18n';
import { Account } from './pages/Account';
import { Activity } from './pages/Activity';
import { Dashboard } from './pages/Dashboard';
import { Login } from './pages/Login';
import { Nodes } from './pages/Nodes';
import { ServerView } from './pages/ServerView';
import { Settings } from './pages/Settings';
import { Streamers } from './pages/Streamers';
import { Users } from './pages/Users';
import { PRESET_EGG_KEY } from './components/CreateServerForm';
import { TwoFactorGate } from './components/TwoFactorGate';

const RTMP_RELAY_EGG_NAME = 'RTMP Relay (offload stream encoding)';

interface StoredUser {
  id: number;
  email: string;
  username: string;
}

type View = 'servers' | 'streamers' | 'nodes' | 'settings' | 'activity' | 'account' | 'users';

const VIEWS: View[] = ['servers', 'streamers', 'nodes', 'settings', 'activity', 'account', 'users'];

function loadUser(): StoredUser | null {
  const raw = localStorage.getItem('user');
  if (!raw) return null;
  try {
    return JSON.parse(raw) as StoredUser;
  } catch {
    return null;
  }
}

interface Location {
  view: View;
  activeServer: string | null;
}

// The URL is the source of truth for which screen is showing, so a reload (or
// a bookmark, or the browser back button) lands back where the user was
// instead of always bouncing to the servers dashboard.
function parseLocation(): Location {
  const path = window.location.pathname;
  const serverMatch = path.match(/^\/servers\/([^/]+)\/?$/);
  if (serverMatch) return { view: 'servers', activeServer: decodeURIComponent(serverMatch[1]) };

  const segment = path.replace(/^\/+/, '').split('/')[0];
  if ((VIEWS as string[]).includes(segment)) return { view: segment as View, activeServer: null };

  return { view: 'servers', activeServer: null };
}

function pathFor(loc: Location): string {
  if (loc.activeServer) return `/servers/${encodeURIComponent(loc.activeServer)}`;
  return loc.view === 'servers' ? '/' : `/${loc.view}`;
}

export function App() {
  const [user, setUser] = useState<StoredUser | null>(() => loadUser());
  const [{ view, activeServer }, setLocation] = useState<Location>(() => parseLocation());
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [mustSetup2FA, setMustSetup2FA] = useState(false);

  useEffect(() => {
    if (!user) return;
    api.me().then((me) => setMustSetup2FA(me.must_setup_2fa)).catch(() => {});
  }, [user]);

  useEffect(() => {
    const onPopState = () => setLocation(parseLocation());
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  function navigate(next: Location) {
    const path = pathFor(next);
    if (path !== window.location.pathname) {
      window.history.pushState(null, '', path);
    }
    setLocation(next);
  }

  function handleLogout() {
    clearTokens();
    setUser(null);
    setMustSetup2FA(false);
    navigate({ view: 'servers', activeServer: null });
  }

  function goTo(next: View) {
    navigate({ view: next, activeServer: null });
    setMobileNavOpen(false);
  }

  function openServer(uuid: string) {
    navigate({ view, activeServer: uuid });
  }

  function handleCreateStreaming() {
    sessionStorage.setItem(PRESET_EGG_KEY, RTMP_RELAY_EGG_NAME);
    goTo('servers');
  }

  function closeServer() {
    navigate({ view, activeServer: null });
  }

  if (!user) {
    return <Login onLoggedIn={() => setUser(loadUser())} />;
  }

  if (mustSetup2FA) {
    return <TwoFactorGate onVerified={() => setMustSetup2FA(false)} onLogout={handleLogout} />;
  }

  return (
    <div id="page-dashboard" className="page active">
      <div className="ambient">
        <div className="blob b1" />
        <div className="blob b2" />
      </div>

      <header className="topbar">
        <button
          className="mobile-nav-toggle"
          onClick={() => setMobileNavOpen((v) => !v)}
          aria-label={t('app.toggleNav')}
        >
          ☰
        </button>
        <div className="topbar-logo">
          <span className="name">Power</span>
          <span className="tag">Node</span>
        </div>
        <div className="topbar-sep" />
        <nav className="breadcrumb">
          <span className={activeServer ? '' : 'bc-cur'} onClick={() => goTo('servers')}>
            {view === 'nodes'
              ? t('nav.nodes')
              : view === 'settings'
                ? t('nav.settings')
                : view === 'activity'
                  ? t('nav.activity')
                  : view === 'account'
                    ? t('nav.account')
                    : view === 'users'
                      ? t('nav.users')
                      : view === 'streamers'
                        ? t('nav.streamers')
                        : t('nav.dashboard')}
          </span>
          {activeServer && (
            <>
              <span className="bc-sep">/</span>
              <span className="bc-cur">{activeServer.slice(0, 8)}</span>
            </>
          )}
        </nav>
        <div className="topbar-right">
          <button
            className="topbar-btn lang-switch"
            onClick={() => setLocale(locale === 'ru' ? 'en' : 'ru')}
            title={locale === 'ru' ? t('app.switchToEnglish') : t('app.switchToRussian')}
          >
            {locale === 'ru' ? 'EN' : 'RU'}
          </button>
          <div className="user-chip" onClick={() => goTo('account')} style={{ cursor: 'pointer' }}>
            <div className="user-ava">{user.username.slice(0, 1).toUpperCase()}</div>
            <span className="user-name">{user.username}</span>
          </div>
        </div>
      </header>

      <div className="shell-body">
        <div
          className={`sidebar-backdrop ${mobileNavOpen ? 'show' : ''}`}
          onClick={() => setMobileNavOpen(false)}
        />
        <aside className={`sidebar ${mobileNavOpen ? 'mobile-open' : ''}`}>
          <div className="nav-section">
            <div className="nav-section-label">{t('nav.overview')}</div>
            <div
              className={`nav-item ${view === 'servers' ? 'active' : ''}`}
              onClick={() => goTo('servers')}
            >
              <span className="nav-icon">▦</span> {t('nav.servers')}
            </div>
          </div>
          <div className="nav-section">
            <div className="nav-section-label">{t('nav.streaming')}</div>
            <div
              className={`nav-item ${view === 'streamers' ? 'active' : ''}`}
              onClick={() => goTo('streamers')}
            >
              <span className="nav-icon">▶</span> {t('nav.streamers')}
            </div>
          </div>
          <div className="nav-section">
            <div className="nav-section-label">{t('nav.admin')}</div>
            <div
              className={`nav-item ${view === 'nodes' ? 'active' : ''}`}
              onClick={() => goTo('nodes')}
            >
              <span className="nav-icon">◆</span> {t('nav.nodes')}
            </div>
            <div
              className={`nav-item ${view === 'users' ? 'active' : ''}`}
              onClick={() => goTo('users')}
            >
              <span className="nav-icon">☺</span> {t('nav.users')}
            </div>
            <div
              className={`nav-item ${view === 'activity' ? 'active' : ''}`}
              onClick={() => goTo('activity')}
            >
              <span className="nav-icon">☰</span> {t('nav.activity')}
            </div>
            <div
              className={`nav-item ${view === 'account' ? 'active' : ''}`}
              onClick={() => goTo('account')}
            >
              <span className="nav-icon">◈</span> {t('nav.account')}
            </div>
          </div>
          <div className="sidebar-footer">
            <div
              className={`nav-item ${view === 'settings' ? 'active' : ''}`}
              onClick={() => goTo('settings')}
            >
              <span className="nav-icon">⚙</span> {t('nav.settings')}
            </div>
            <div className="nav-item logout-item" onClick={handleLogout}>
              <span className="nav-icon">⏻</span> {t('nav.logout')}
            </div>
          </div>
        </aside>

        <main className="main">
          {activeServer ? (
            <ServerView uuid={activeServer} onBack={closeServer} />
          ) : view === 'streamers' ? (
            <Streamers onManage={openServer} onCreateStreaming={handleCreateStreaming} />
          ) : view === 'nodes' ? (
            <Nodes />
          ) : view === 'users' ? (
            <Users />
          ) : view === 'settings' ? (
            <Settings />
          ) : view === 'activity' ? (
            <Activity />
          ) : view === 'account' ? (
            <Account />
          ) : (
            <Dashboard onManage={openServer} />
          )}
        </main>
      </div>
    </div>
  );
}
