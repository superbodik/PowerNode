import { useMemo, useState } from 'react';

interface Props {
  // Optional/nullable because callers may hand back a summary parsed
  // straight from localStorage -- an entry saved before this field existed
  // simply won't have it, so this can't assume the array is always there.
  values: number[] | undefined | null;
  color?: string;
  height?: number;
  unit?: string;
}

const VIEW_WIDTH = 240;
const PAD_Y = 5;

// A compact trend line for a stat tile -- single series, so no legend (the
// card title already says what's plotted). Thin 2px line, a light area
// wash, an end-dot marking the latest value, and a minimal hover readout
// so the exact value at any point is still reachable, not just eyeballed.
export function Sparkline({ values, color = 'var(--pink-b)', height = 44, unit = 'kbps' }: Props) {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  const safeValues = values ?? [];

  const { linePath, areaPath, points, min, max } = useMemo(() => {
    if (safeValues.length < 2) {
      return { linePath: '', areaPath: '', points: [] as { x: number; y: number }[], min: 0, max: 0 };
    }
    const lo = Math.min(...safeValues);
    const hi = Math.max(...safeValues);
    const span = hi - lo || 1;
    const usableHeight = height - PAD_Y * 2;
    const pts = safeValues.map((v, i) => ({
      x: (i / (safeValues.length - 1)) * VIEW_WIDTH,
      y: PAD_Y + usableHeight - ((v - lo) / span) * usableHeight,
    }));
    const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' ');
    const area = `${line} L${pts[pts.length - 1].x.toFixed(2)},${height} L0,${height} Z`;
    return { linePath: line, areaPath: area, points: pts, min: lo, max: hi };
  }, [safeValues, height]);

  if (safeValues.length < 2) {
    return null;
  }

  const last = points[points.length - 1];
  const hovered = hoverIndex != null ? points[hoverIndex] : null;

  function handleMove(e: React.PointerEvent<SVGSVGElement>) {
    const rect = e.currentTarget.getBoundingClientRect();
    const ratio = (e.clientX - rect.left) / rect.width;
    const idx = Math.round(ratio * (points.length - 1));
    setHoverIndex(Math.max(0, Math.min(points.length - 1, idx)));
  }

  return (
    <div style={{ position: 'relative' }}>
      <svg
        viewBox={`0 0 ${VIEW_WIDTH} ${height}`}
        width="100%"
        height={height}
        preserveAspectRatio="none"
        onPointerMove={handleMove}
        onPointerLeave={() => setHoverIndex(null)}
        style={{ display: 'block', cursor: 'crosshair' }}
      >
        <path d={areaPath} fill={color} opacity={0.1} stroke="none" />
        <path d={linePath} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
        <circle cx={last.x} cy={last.y} r={4} fill={color} stroke="var(--bg)" strokeWidth={2} />
        {hovered && (
          <>
            <line x1={hovered.x} y1={0} x2={hovered.x} y2={height} stroke="var(--border-h)" strokeWidth={1} />
            <circle cx={hovered.x} cy={hovered.y} r={4} fill={color} stroke="var(--bg)" strokeWidth={2} />
          </>
        )}
      </svg>
      {hoverIndex != null && (
        <div
          className="sparkline-tooltip"
          style={{ left: `${(hoverIndex / (points.length - 1)) * 100}%` }}
        >
          {Math.round(safeValues[hoverIndex])} {unit}
        </div>
      )}
      <div className="sparkline-range">
        <span>{Math.round(min)}</span>
        <span>{Math.round(max)} {unit}</span>
      </div>
    </div>
  );
}
