import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';

interface CurrencyTotal {
  currency: string;
  total_cents: number;
  count: number;
}

interface AnalyticsData {
  twitch_connected: boolean;
  live: boolean;
  viewer_count?: number;
  donations_today: CurrencyTotal[];
  donations_all_time: CurrencyTotal[];
}

function formatTotals(totals: CurrencyTotal[]): string {
  if (totals.length === 0) return '—';
  return totals.map((ct) => `${(ct.total_cents / 100).toFixed(2)} ${ct.currency.toUpperCase()}`).join(' + ');
}

function donationCount(totals: CurrencyTotal[]): number {
  return totals.reduce((sum, ct) => sum + ct.count, 0);
}

interface Props {
  onBack: () => void;
}

export function Analytics({ onBack }: Props) {
  const [data, setData] = useState<AnalyticsData | null>(null);
  const [error, setError] = useState<string | null>(null);

  function refresh() {
    api
      .getStreamerAnalytics()
      .then(setData)
      .catch((err) => setError(err instanceof Error ? err.message : String(err)));
  }

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 30000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>{t('analytics.title')}</h1>
        <p>{t('analytics.subtitle')}</p>
      </div>

      <button className="btn-sm" onClick={onBack} style={{ marginBottom: 20 }}>
        {t('analytics.back')}
      </button>

      {error && <div className="login-error show" style={{ marginBottom: 16 }}>{error}</div>}

      {!data ? (
        <p className="srv-desc">{t('common.loading')}</p>
      ) : (
        <>
          <div className="servers-grid" style={{ marginBottom: 20 }}>
            <div className="settings-card">
              <div className="settings-card-title">{t('analytics.viewersTile')}</div>
              {!data.twitch_connected ? (
                <p className="srv-desc">{t('analytics.viewersNoTwitch')}</p>
              ) : (
                <>
                  <div className={`status-badge ${data.live ? 'online' : 'offline'}`} style={{ marginBottom: 10 }}>
                    <span className="dot" />
                    {data.live ? t('status.running') : t('status.offline')}
                  </div>
                  <span className="kpi-value">{data.live ? (data.viewer_count ?? 0) : '—'}</span>
                  <p className="srv-desc" style={{ marginTop: 6 }}>{t('analytics.viewersHint')}</p>
                </>
              )}
            </div>

            <div className="settings-card">
              <div className="settings-card-title">{t('analytics.donationsTodayTile')}</div>
              <span className="kpi-value">{formatTotals(data.donations_today)}</span>
              <p className="srv-desc" style={{ marginTop: 6 }}>
                {t('analytics.donationsCount', { count: donationCount(data.donations_today) })}
              </p>
            </div>

            <div className="settings-card">
              <div className="settings-card-title">{t('analytics.donationsAllTimeTile')}</div>
              <span className="kpi-value">{formatTotals(data.donations_all_time)}</span>
              <p className="srv-desc" style={{ marginTop: 6 }}>
                {t('analytics.donationsCount', { count: donationCount(data.donations_all_time) })}
              </p>
            </div>

            <div className="settings-card">
              <div className="settings-card-title">{t('analytics.chatTile')}</div>
              <p className="srv-desc">{t('analytics.chatComingSoon')}</p>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
