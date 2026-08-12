import { useEffect, useRef, useState } from 'react';

const MAX_SAMPLES = 120;

export interface StreamSessionSummary {
  startedAt: number;
  endedAt: number;
  peakKbps: number;
  avgKbps: number;
  samples: number[];
}

interface CurrentSession {
  startedAt: number;
  peakKbps: number;
  sumKbps: number;
  sampleCount: number;
  samples: number[];
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

// Halves the series by dropping every other point once it grows past the
// cap -- keeps the sparkline light in both render cost and localStorage
// size without needing a proper streaming-decimation algorithm for what's
// ultimately a glanceable trend line, not an analytical chart.
function pushSample(samples: number[], value: number): number[] {
  const next = samples.concat(value);
  if (next.length <= MAX_SAMPLES) return next;
  return next.filter((_, i) => i % 2 === 0);
}

// Tracks peak/average bitrate (and a decimated sample series for the
// sparkline) for the stream currently in progress, and keeps a summary of
// the last completed one in localStorage so it's still there after a page
// reload. Deliberately not a backend feature: this is per-browser
// convenience, not a source of truth anyone else needs to read.
export function useStreamSessionStats(
  uuid: string,
  signalLive: boolean,
  liveSince: number | null,
  inboundKbps: number,
) {
  const [lastSession, setLastSession] = useState<StreamSessionSummary | null>(() => loadLastSession(uuid));
  const [currentPeakKbps, setCurrentPeakKbps] = useState(0);
  const [currentAvgKbps, setCurrentAvgKbps] = useState(0);
  const [currentSamples, setCurrentSamples] = useState<number[]>([]);
  const currentRef = useRef<CurrentSession | null>(null);

  useEffect(() => {
    setLastSession(loadLastSession(uuid));
    currentRef.current = null;
    setCurrentPeakKbps(0);
    setCurrentAvgKbps(0);
    setCurrentSamples([]);
  }, [uuid]);

  useEffect(() => {
    if (signalLive && liveSince) {
      if (!currentRef.current || currentRef.current.startedAt !== liveSince) {
        currentRef.current = { startedAt: liveSince, peakKbps: 0, sumKbps: 0, sampleCount: 0, samples: [] };
      }
      const session = currentRef.current;
      session.peakKbps = Math.max(session.peakKbps, inboundKbps);
      session.sumKbps += inboundKbps;
      session.sampleCount += 1;
      session.samples = pushSample(session.samples, inboundKbps);
      setCurrentPeakKbps(session.peakKbps);
      setCurrentAvgKbps(Math.round(session.sumKbps / session.sampleCount));
      setCurrentSamples(session.samples);
      return;
    }

    const session = currentRef.current;
    if (!session) return;
    currentRef.current = null;
    setCurrentPeakKbps(0);
    setCurrentAvgKbps(0);
    setCurrentSamples([]);

    const endedAt = Date.now();
    if (endedAt - session.startedAt < 5000) return; // too short to mean anything
    const summary: StreamSessionSummary = {
      startedAt: session.startedAt,
      endedAt,
      peakKbps: session.peakKbps,
      avgKbps: session.sampleCount ? Math.round(session.sumKbps / session.sampleCount) : 0,
      samples: session.samples,
    };
    try {
      localStorage.setItem(storageKey(uuid), JSON.stringify(summary));
    } catch {
      /* storage full/unavailable -- not worth failing over */
    }
    setLastSession(summary);
  }, [uuid, signalLive, liveSince, inboundKbps]);

  return { lastSession, currentPeakKbps, currentAvgKbps, currentSamples };
}
