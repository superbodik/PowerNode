import { useEffect, useRef, useState } from 'react';
import { connectServerSocketWithRetry } from '../api/client';
import type { ResourceStats } from '../types';

export interface StreamSignal {
  inboundKbps: number;
  signalLive: boolean;
  liveSince: number | null;
}

// Derives "is this server actually receiving a stream right now" purely from
// the container's own inbound network byte counter, already pushed every ~2s
// over the stats socket -- no mediamtx-specific API call needed (checked its
// OpenAPI schema: no such live/bitrate field exists there anyway). Docker
// hands back cumulative byte counters in bursts rather than an even drip, so
// raw two-sample deltas are smoothed with an EMA instead of shown directly.
export function useStreamSignal(uuid: string | null): StreamSignal {
  const [inboundKbps, setInboundKbps] = useState(0);
  const [signalLive, setSignalLive] = useState(false);
  const [liveSince, setLiveSince] = useState<number | null>(null);
  const prevNetSampleRef = useRef<{ rx: number; at: number } | null>(null);
  const smoothedKbpsRef = useRef<number | null>(null);

  useEffect(() => {
    prevNetSampleRef.current = null;
    smoothedKbpsRef.current = null;
    setInboundKbps(0);
    setSignalLive(false);
    setLiveSince(null);
    if (!uuid) return;

    return connectServerSocketWithRetry<ResourceStats>(uuid, (live) => {
      const now = Date.now();
      const prev = prevNetSampleRef.current;
      prevNetSampleRef.current = { rx: live.network_rx, at: now };
      if (!prev) return;

      const deltaBytes = live.network_rx - prev.rx;
      const deltaSeconds = (now - prev.at) / 1000;
      if (deltaBytes < 0 || deltaSeconds <= 0) {
        smoothedKbpsRef.current = null;
        setInboundKbps(0);
        setSignalLive(false);
        setLiveSince(null);
        return;
      }

      const rawKbps = (deltaBytes * 8) / 1000 / deltaSeconds;
      const isLive = rawKbps > 5 && live.state === 'running';
      smoothedKbpsRef.current = !isLive
        ? null
        : smoothedKbpsRef.current == null
          ? rawKbps
          : smoothedKbpsRef.current * 0.6 + rawKbps * 0.4;
      setInboundKbps(Math.round(smoothedKbpsRef.current ?? 0));
      setSignalLive(isLive);
      setLiveSince((prevSince) => (isLive ? (prevSince ?? now) : null));
    });
  }, [uuid]);

  return { inboundKbps, signalLive, liveSince };
}
