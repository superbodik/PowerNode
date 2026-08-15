import { useEffect, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import { Sparkline } from '../components/Sparkline';
import { formatDuration, formatRelativeTime } from '../utils/format';
import type { StreamSession } from '../types';

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
  chat_messages_today: number;
}

function formatTotals(totals: CurrencyTotal[]): string {
  if (totals.length === 0) return '—';
  return totals.map((ct) => `${(ct.total_cents / 100).toFixed(2)} ${ct.currency.toUpperCase()}`).join(' + ');
}

function donationCount(totals: CurrencyTotal[]): number {
  return totals.reduce((sum, ct) => sum + ct.count, 0);
}

function sessionDuration(session: StreamSession): number {
  const start = new Date(session.started_at).getTime();
  const end = session.ended_at ? new Date(session.ended_at).getTime() : Date.now();
  return (end - start) / 1000;
}

interface Props {
  onBack: () => void;
}

export function Analytics({ onBack }: Props) {
  const [data, setData] = useState<AnalyticsData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [sessions, setSessions] = useState<StreamSession[]>([]);
  const [expanded, setExpanded] = useState<Record<number, StreamSession>>({});
  const [openID, setOpenID] = useState<number | null>(null);

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

  useEffect(() => {
    api.listStreamSessions().then(setSessions).catch(() => {});
  }, []);

  function toggleSession(id: number) {
    if (openID === id) {
      setOpenID(null);
      return;
    }
    setOpenID(id);
    if (!expanded[id]) {
      api
        .getStreamSession(id)
        .then((full) => setExpanded((cur) => ({ ...cur, [id]: full })))
        .catch(() => {});
    }
  }

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
              <span className="kpi-value">{data.chat_messages_today}</span>
              <p className="srv-desc" style={{ marginTop: 6 }}>{t('analytics.chatMessagesHint')}</p>
            </div>
          </div>

          <div className="dash-head" style={{ marginTop: 8 }}>
            <h1 style={{ fontSize: 20 }}>{t('analytics.historyTitle')}</h1>
            <p>{t('analytics.historyHint')}</p>
          </div>

          {sessions.length === 0 ? (
            <p className="srv-desc" style={{ marginTop: 12 }}>{t('analytics.noSessionsYet')}</p>
          ) : (
            <div style={{ marginTop: 12 }}>
              {sessions.map((session) => {
                const full = expanded[session.id];
                const open = openID === session.id;
                return (
                  <div className="settings-card" key={session.id} style={{ marginBottom: 10 }}>
                    <div
                      style={{ display: 'flex', alignItems: 'center', gap: 16, cursor: 'pointer', flexWrap: 'wrap' }}
                      onClick={() => toggleSession(session.id)}
                    >
                      <strong>{formatRelativeTime(new Date(session.started_at).getTime())}</strong>
                      <span className="srv-desc">{formatDuration(sessionDuration(session))}</span>
                      <span className="srv-desc">{t('analytics.sessionPeak', { count: session.peak_viewers })}</span>
                      <span className="srv-desc">{t('analytics.sessionChat', { count: session.chat_messages })}</span>
                      <span className="srv-desc">{formatTotals(session.donation_total)}</span>
                      {!session.ended_at && (
                        <span className="status-badge online" style={{ marginLeft: 'auto' }}>
                          <span className="dot" />
                          {t('status.running')}
                        </span>
                      )}
                    </div>

                    {open && (
                      <div style={{ marginTop: 14 }}>
                        {!full ? (
                          <p className="srv-desc">{t('common.loading')}</p>
                        ) : (
                          <>
                            {full.samples && full.samples.length > 1 && (
                              <Sparkline values={full.samples.map((s) => s.viewers)} unit="viewers" />
                            )}
                            {full.donations && full.donations.length > 0 ? (
                              <div style={{ marginTop: 10 }}>
                                {full.donations.map((d, i) => (
                                  <div
                                    key={i}
                                    style={{ display: 'flex', gap: 10, fontSize: 12, padding: '4px 0', borderTop: i > 0 ? '1px solid rgba(255,255,255,.06)' : undefined }}
                                  >
                                    <span style={{ color: 'var(--pink-b, #e8a8b8)', fontWeight: 700 }}>{d.donor_name || t('analytics.anonymousDonor')}</span>
                                    <span>{(d.amount_cents / 100).toFixed(2)} {d.currency.toUpperCase()}</span>
                                    <span className="srv-desc" style={{ flex: 1 }}>{d.message}</span>
                                  </div>
                                ))}
                              </div>
                            ) : (
                              <p className="srv-desc" style={{ marginTop: 10 }}>{t('analytics.noDonationsThisSession')}</p>
                            )}
                          </>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </>
      )}
    </div>
  );
}
