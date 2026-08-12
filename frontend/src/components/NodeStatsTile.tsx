import { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import { Sparkline } from './Sparkline';
import type { Node, NodeStatus } from '../types';

const POLL_MS = 5000;
const MAX_SAMPLES = 60; // ~5 minutes at one sample every 5s

interface Props {
  node: Node;
}

// A live glance at a node's real host RAM (not the admin-set memory_mb
// allocatable-capacity field used for oversell math elsewhere on this page
// -- a different concept entirely). Polls the daemon directly through the
// existing /nodes/{id}/status check-status endpoint rather than adding a
// new one, and keeps a short client-side rolling buffer for the sparkline
// since nothing here needs to be persisted server-side.
export function NodeStatsTile({ node }: Props) {
  const [status, setStatus] = useState<NodeStatus | null>(null);
  const [usedSamples, setUsedSamples] = useState<number[]>([]);
  const samplesRef = useRef<number[]>([]);

  useEffect(() => {
    let cancelled = false;
    samplesRef.current = [];
    setUsedSamples([]);
    setStatus(null);

    function poll() {
      api
        .checkNodeStatus(node.id)
        .then((s) => {
          if (cancelled) return;
          setStatus(s);
          if (s.online && s.mem_total_mb && s.mem_available_mb != null) {
            const usedMB = s.mem_total_mb - s.mem_available_mb;
            const next = samplesRef.current.concat(usedMB).slice(-MAX_SAMPLES);
            samplesRef.current = next;
            setUsedSamples(next);
          }
        })
        .catch(() => {
          if (!cancelled) setStatus({ online: false });
        });
    }

    poll();
    const interval = setInterval(poll, POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [node.id]);

  const usedMB = status?.mem_total_mb && status.mem_available_mb != null ? status.mem_total_mb - status.mem_available_mb : null;
  const usedPct = usedMB != null && status?.mem_total_mb ? Math.round((usedMB / status.mem_total_mb) * 100) : null;

  return (
    <div className="settings-card">
      <div
        className="settings-card-title"
        style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}
      >
        <span>{node.name}</span>
        <div className={`status-badge ${status?.online ? 'online' : 'offline'}`}>
          <span className="dot" />
          {status === null ? t('common.loading') : status.online ? t('status.running') : t('status.offline')}
        </div>
      </div>

      {status?.online ? (
        <>
          <div className="kpi-row">
            <div className="kpi-tile">
              <span className="kpi-value">
                {usedMB != null ? (usedMB / 1024).toFixed(1) : '—'} <span className="kpi-unit">GB</span>
              </span>
              <span className="kpi-label">{t('nodes.statsRamUsed')}</span>
            </div>
            <div className="kpi-tile">
              <span className="kpi-value">
                {status.mem_total_mb ? (status.mem_total_mb / 1024).toFixed(1) : '—'} <span className="kpi-unit">GB</span>
              </span>
              <span className="kpi-label">{t('nodes.statsRamTotal')}</span>
            </div>
            <div className="kpi-tile">
              <span className="kpi-value">{status.cpu_cores ?? '—'}</span>
              <span className="kpi-label">{t('nodes.statsCpuCores')}</span>
            </div>
          </div>
          {usedPct != null && (
            <Sparkline values={usedSamples} color={usedPct >= 85 ? 'var(--red)' : 'var(--pink-b)'} unit="MB" />
          )}
        </>
      ) : (
        <p className="srv-desc">{status?.error || t('nodes.statsOffline')}</p>
      )}
    </div>
  );
}
