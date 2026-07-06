import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ActivityEntry } from '../types';

export function Activity() {
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .listActivity()
      .then(setEntries)
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  }, []);

  if (error) return <p className="srv-desc">Only admins can view the activity log.</p>;

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>Activity</h1>
        <p>Recent actions across the panel.</p>
      </div>

      {loading ? (
        <p className="srv-desc">Loading…</p>
      ) : (
        <div className="act-table">
          <div className="act-head">
            <span>User</span>
            <span>Event</span>
            <span>IP</span>
            <span>Time</span>
          </div>
          {entries.map((entry) => (
            <div className="act-row" key={entry.id}>
              <div className="act-user">
                <div className="act-ava">
                  {(entry.username ?? '?').slice(0, 1).toUpperCase()}
                </div>
                <span>{entry.username ?? 'system'}</span>
              </div>
              <span className="act-event">{entry.event}</span>
              <span className="act-ip">{entry.ip_address ?? '—'}</span>
              <span className="act-time">{new Date(entry.created_at).toLocaleString()}</span>
            </div>
          ))}
          {entries.length === 0 && (
            <p className="srv-desc" style={{ padding: 16 }}>
              No activity yet.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
