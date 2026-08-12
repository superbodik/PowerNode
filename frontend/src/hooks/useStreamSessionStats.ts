import { useEffect, useRef, useState } from 'react';

export interface StreamSessionSummary {
  startedAt: number;
  endedAt: number;
  peakKbps: number;
  avgKbps: number;
}

interface CurrentSession {
  startedAt: number;
  peakKbps: number;
  sumKbps: number;
  samples: number;
}

function storageKey(uuid: string): string {
  return `pn_stream_last_session_${uuid}`;
}

function loadLastSession(uuid: string): StreamSessionSummary | null {
  try {
    const raw = localStorage.getItem(storageKey(uuid));
    return raw ? (JSON.parse(raw) as StreamSessionSummary) : null;
  } catch {
    return null;
  }
}

// Tracks peak/average bitrate for the stream currently in progress, and
// keeps a summary of the last completed one in localStorage so it's still
// there after a page reload. Deliberately not a backend feature: this is
// per-browser convenience, not a source of truth anyone else needs to read.
export function useStreamSessionStats(
  uuid: string,
  signalLive: boolean,
  liveSince: number | null,
  inboundKbps: number,
) {
  const [lastSession, setLastSession] = useState<StreamSessionSummary | null>(() => loadLastSession(uuid));
  const [currentPeakKbps, setCurrentPeakKbps] = useState(0);
  const [currentAvgKbps, setCurrentAvgKbps] = useState(0);
  const currentRef = useRef<CurrentSession | null>(null);

  useEffect(() => {
    setLastSession(loadLastSession(uuid));
    currentRef.current = null;
    setCurrentPeakKbps(0);
    setCurrentAvgKbps(0);
  }, [uuid]);

  useEffect(() => {
    if (signalLive && liveSince) {
      if (!currentRef.current || currentRef.current.startedAt !== liveSince) {
        currentRef.current = { startedAt: liveSince, peakKbps: 0, sumKbps: 0, samples: 0 };
      }
      const session = currentRef.current;
      session.peakKbps = Math.max(session.peakKbps, inboundKbps);
      session.sumKbps += inboundKbps;
      session.samples += 1;
      setCurrentPeakKbps(session.peakKbps);
      setCurrentAvgKbps(Math.round(session.sumKbps / session.samples));
      return;
    }

    const session = currentRef.current;
    if (!session) return;
    currentRef.current = null;
    setCurrentPeakKbps(0);
    setCurrentAvgKbps(0);

    const endedAt = Date.now();
    if (endedAt - session.startedAt < 5000) return; // too short to mean anything
    const summary: StreamSessionSummary = {
      startedAt: session.startedAt,
      endedAt,
      peakKbps: session.peakKbps,
      avgKbps: session.samples ? Math.round(session.sumKbps / session.samples) : 0,
    };
    try {
      localStorage.setItem(storageKey(uuid), JSON.stringify(summary));
    } catch {
      /* storage full/unavailable -- not worth failing over */
    }
    setLastSession(summary);
  }, [uuid, signalLive, liveSince, inboundKbps]);

  return { lastSession, currentPeakKbps, currentAvgKbps };
}
