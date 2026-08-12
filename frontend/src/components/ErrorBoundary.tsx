import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';
import { t } from '../i18n';

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// A single render-time exception anywhere in the tree used to take down
// the whole app -- and with it, every open WebSocket, since the root
// unmounts. Confirmed in production: a bad localStorage read in one stat
// widget crashed the entire dashboard for every server on the page.
// React error boundaries only exist as class components; there's no hook
// equivalent.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Unhandled error in page:', error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="view active" style={{ padding: 40, textAlign: 'center' }}>
          <p className="srv-desc" style={{ marginBottom: 16 }}>
            {t('app.somethingBroke')}
          </p>
          <button className="btn-sm" onClick={() => window.location.reload()}>
            {t('app.reload')}
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
