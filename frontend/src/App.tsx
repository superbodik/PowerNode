import { useState } from 'react';
import { Dashboard } from './pages/Dashboard';
import { Login } from './pages/Login';
import { Nodes } from './pages/Nodes';
import { Settings } from './pages/Settings';

interface StoredUser {
  id: number;
  email: string;
  username: string;
}

type View = 'servers' | 'nodes' | 'settings';

function loadUser(): StoredUser | null {
  const raw = localStorage.getItem('user');
  if (!raw) return null;
  try {
    return JSON.parse(raw) as StoredUser;
  } catch {
    return null;
  }
}

export function App() {
  const [user, setUser] = useState<StoredUser | null>(() => loadUser());
  const [view, setView] = useState<View>('servers');
  const [activeServer, setActiveServer] = useState<string | null>(null);

  function handleLogout() {
    localStorage.removeItem('access_token');
    localStorage.removeItem('user');
    setUser(null);
  }

  function goTo(next: View) {
    setActiveServer(null);
    setView(next);
  }

  if (!user) {
    return <Login onLoggedIn={() => setUser(loadUser())} />;
  }

  return (
    <div id="page-dashboard" className="page active">
      <div className="ambient">
        <div className="blob b1" />
        <div className="blob b2" />
      </div>

      <header className="topbar">
        <div className="topbar-logo">
          <span className="name">Panel</span>
          <span className="tag">Control</span>
        </div>
        <div className="topbar-sep" />
        <nav className="breadcrumb">
          <span className={activeServer ? '' : 'bc-cur'} onClick={() => goTo('servers')}>
            {view === 'nodes' ? 'Nodes' : view === 'settings' ? 'Settings' : 'Dashboard'}
          </span>
          {activeServer && (
            <>
              <span className="bc-sep">/</span>
              <span className="bc-cur">{activeServer}</span>
            </>
          )}
        </nav>
        <div className="topbar-right">
          <div className="user-chip">
            <div className="user-ava">{user.username.slice(0, 1).toUpperCase()}</div>
            <span className="user-name">{user.username}</span>
          </div>
        </div>
      </header>

      <div className="shell-body">
        <aside className="sidebar">
          <div className="nav-section">
            <div className="nav-section-label">Overview</div>
            <div
              className={`nav-item ${view === 'servers' ? 'active' : ''}`}
              onClick={() => goTo('servers')}
            >
              <span className="nav-icon">▦</span> Servers
            </div>
          </div>
          <div className="nav-section">
            <div className="nav-section-label">Admin</div>
            <div
              className={`nav-item ${view === 'nodes' ? 'active' : ''}`}
              onClick={() => goTo('nodes')}
            >
              <span className="nav-icon">◆</span> Nodes
            </div>
            <div className="nav-item">
              <span className="nav-icon">☰</span> Activity
            </div>
            <div
              className={`nav-item ${view === 'settings' ? 'active' : ''}`}
              onClick={() => goTo('settings')}
            >
              <span className="nav-icon">⚙</span> Settings
            </div>
          </div>
          <div className="sidebar-footer">
            <div className="nav-item logout-item" onClick={handleLogout}>
              <span className="nav-icon">⏻</span> Log out
            </div>
          </div>
        </aside>

        <main className="main">
          {view === 'nodes' ? (
            <Nodes />
          ) : view === 'settings' ? (
            <Settings />
          ) : (
            <Dashboard onManage={setActiveServer} />
          )}
        </main>
      </div>
    </div>
  );
}
