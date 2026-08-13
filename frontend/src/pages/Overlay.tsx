import { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import { t } from '../i18n';
import type { OverlayWidget, OverlayWidgetType } from '../types';

interface Props {
  // Present when this page was opened via the standalone moderator link
  // (/overlay-editor?token=...) rather than from inside the logged-in panel
  // -- undefined means "authenticate as the logged-in owner instead".
  token?: string;
  onBack?: () => void;
}

const WIDGET_DEFAULTS: Record<OverlayWidgetType, { width: number; height: number; config: Record<string, unknown> }> = {
  chat: { width: 26, height: 55, config: {} },
  text: { width: 30, height: 10, config: { text: 'Text' } },
  image: { width: 20, height: 20, config: { url: '' } },
  viewer_count: { width: 16, height: 12, config: {} },
  donation_total: { width: 16, height: 12, config: {} },
};

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), Math.max(min, max));
}

function widgetLabel(type: OverlayWidgetType): string {
  switch (type) {
    case 'chat':
      return t('overlay.widgetChat');
    case 'text':
      return t('overlay.widgetText');
    case 'image':
      return t('overlay.widgetImage');
    case 'viewer_count':
      return t('overlay.widgetViewerCount');
    case 'donation_total':
      return t('overlay.widgetDonationTotal');
  }
}

export function Overlay({ token, onBack }: Props) {
  const [widgets, setWidgets] = useState<OverlayWidget[]>([]);
  const [moderatorUrl, setModeratorUrl] = useState('');
  const [renderUrl, setRenderUrl] = useState('');
  const [selected, setSelected] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const canvasRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    api
      .getOverlayLayout(token)
      .then((layout) => {
        setWidgets(layout.widgets);
        setModeratorUrl(layout.moderator_url);
        setRenderUrl(layout.render_url);
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  }, [token]);

  function addWidget(type: OverlayWidgetType) {
    const d = WIDGET_DEFAULTS[type];
    setWidgets((prev) => {
      const next = [...prev, { widget_type: type, x: 5, y: 5, width: d.width, height: d.height, z_index: prev.length, config: { ...d.config } }];
      setSelected(next.length - 1);
      return next;
    });
  }

  function removeSelected() {
    if (selected === null) return;
    setWidgets((prev) => prev.filter((_, i) => i !== selected));
    setSelected(null);
  }

  function bringSelectedToFront() {
    if (selected === null) return;
    setWidgets((prev) => {
      const maxZ = prev.reduce((m, w) => Math.max(m, w.z_index), 0);
      return prev.map((w, i) => (i === selected ? { ...w, z_index: maxZ + 1 } : w));
    });
  }

  function updateSelectedConfig(patch: Record<string, unknown>) {
    if (selected === null) return;
    setWidgets((prev) => prev.map((w, i) => (i === selected ? { ...w, config: { ...w.config, ...patch } } : w)));
  }

  // Drag/resize handlers are created fresh on each mousedown and capture
  // their own start position + original widget rect in a closure, so the
  // window listener removed in handleUp is always the exact instance that
  // was added -- re-renders during the drag (setWidgets fires on every
  // mousemove) don't touch this closure at all.
  function onHandleDown(e: React.MouseEvent, index: number, mode: 'move' | 'resize') {
    e.preventDefault();
    e.stopPropagation();
    setSelected(index);
    const startX = e.clientX;
    const startY = e.clientY;
    const orig = widgets[index];

    function handleMove(ev: MouseEvent) {
      const canvas = canvasRef.current;
      if (!canvas) return;
      const rect = canvas.getBoundingClientRect();
      const dxPct = ((ev.clientX - startX) / rect.width) * 100;
      const dyPct = ((ev.clientY - startY) / rect.height) * 100;
      setWidgets((prev) =>
        prev.map((w, i) => {
          if (i !== index) return w;
          if (mode === 'move') {
            return {
              ...w,
              x: clamp(orig.x + dxPct, 0, 100 - orig.width),
              y: clamp(orig.y + dyPct, 0, 100 - orig.height),
            };
          }
          return {
            ...w,
            width: clamp(orig.width + dxPct, 5, 100 - orig.x),
            height: clamp(orig.height + dyPct, 5, 100 - orig.y),
          };
        }),
      );
    }

    function handleUp() {
      window.removeEventListener('mousemove', handleMove);
      window.removeEventListener('mouseup', handleUp);
    }

    window.addEventListener('mousemove', handleMove);
    window.addEventListener('mouseup', handleUp);
  }

  async function save() {
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      await api.saveOverlayWidgets(widgets, token);
      setSaved(true);
      setTimeout(() => setSaved(false), 2500);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }

  const sel = selected !== null ? widgets[selected] : null;

  return (
    <div className={token ? 'view active' : 'view active'} style={token ? { maxWidth: 960, margin: '0 auto', padding: 24 } : undefined}>
      <div className="dash-head">
        <h1>{t('overlay.title')}</h1>
        <p>{t('overlay.subtitle')}</p>
      </div>

      {onBack && (
        <button className="btn-sm" onClick={onBack} style={{ marginBottom: 20 }}>
          {t('analytics.back')}
        </button>
      )}

      {error && <div className="login-error show" style={{ marginBottom: 16 }}>{error}</div>}

      {loading ? (
        <p className="srv-desc">{t('common.loading')}</p>
      ) : (
        <>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }}>
            <button className="btn-sm" onClick={() => addWidget('chat')}>+ {t('overlay.widgetChat')}</button>
            <button className="btn-sm" onClick={() => addWidget('viewer_count')}>+ {t('overlay.widgetViewerCount')}</button>
            <button className="btn-sm" onClick={() => addWidget('donation_total')}>+ {t('overlay.widgetDonationTotal')}</button>
            <button className="btn-sm" onClick={() => addWidget('text')}>+ {t('overlay.widgetText')}</button>
            <button className="btn-sm" onClick={() => addWidget('image')}>+ {t('overlay.widgetImage')}</button>
            <div style={{ flex: 1 }} />
            <button className="btn-primary" disabled={saving} onClick={save}>
              {saving ? t('common.loading') : t('overlay.save')}
            </button>
          </div>

          {saved && (
            <p className="srv-desc" style={{ color: 'var(--green, #5fe69a)', marginBottom: 10 }}>
              {t('overlay.saved')}
            </p>
          )}

          <div style={{ display: 'flex', gap: 20, flexWrap: 'wrap', alignItems: 'flex-start' }}>
            <div
              ref={canvasRef}
              onMouseDown={() => setSelected(null)}
              style={{
                position: 'relative',
                width: '100%',
                maxWidth: 720,
                aspectRatio: '16 / 9',
                background:
                  'repeating-linear-gradient(45deg, rgba(255,255,255,.03) 0 10px, rgba(255,255,255,.01) 10px 20px), #0a080c',
                border: '1px solid rgba(232,168,184,.15)',
                borderRadius: 12,
                overflow: 'hidden',
                flex: '2 1 480px',
              }}
            >
              {widgets.map((w, i) => (
                <div
                  key={i}
                  onMouseDown={(e) => onHandleDown(e, i, 'move')}
                  style={{
                    position: 'absolute',
                    left: `${w.x}%`,
                    top: `${w.y}%`,
                    width: `${w.width}%`,
                    height: `${w.height}%`,
                    zIndex: w.z_index,
                    border: selected === i ? '2px solid var(--pink-b, #e8a8b8)' : '1px dashed rgba(255,255,255,.25)',
                    background: 'rgba(232,168,184,.08)',
                    borderRadius: 6,
                    cursor: 'move',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: '#ece4e8',
                    fontSize: 11,
                    textAlign: 'center',
                    padding: 4,
                    boxSizing: 'border-box',
                    userSelect: 'none',
                  }}
                >
                  {widgetLabel(w.widget_type)}
                  <div
                    onMouseDown={(e) => onHandleDown(e, i, 'resize')}
                    style={{
                      position: 'absolute',
                      right: 0,
                      bottom: 0,
                      width: 14,
                      height: 14,
                      background: 'var(--pink-b, #e8a8b8)',
                      cursor: 'nwse-resize',
                      borderRadius: '3px 0 6px 0',
                    }}
                  />
                </div>
              ))}
            </div>

            <div className="settings-card" style={{ flex: '1 1 260px', minWidth: 240 }}>
              <div className="settings-card-title">{t('overlay.propertiesTitle')}</div>
              {!sel ? (
                <p className="srv-desc">{t('overlay.propertiesHint')}</p>
              ) : (
                <>
                  <p className="srv-desc" style={{ marginBottom: 10 }}>{widgetLabel(sel.widget_type)}</p>
                  {sel.widget_type === 'text' && (
                    <div className="sfield" style={{ marginBottom: 10 }}>
                      <label>{t('overlay.textLabel')}</label>
                      <input
                        type="text"
                        value={typeof sel.config.text === 'string' ? sel.config.text : ''}
                        onChange={(e) => updateSelectedConfig({ text: e.target.value })}
                      />
                    </div>
                  )}
                  {sel.widget_type === 'image' && (
                    <div className="sfield" style={{ marginBottom: 10 }}>
                      <label>{t('overlay.imageUrlLabel')}</label>
                      <input
                        type="text"
                        value={typeof sel.config.url === 'string' ? sel.config.url : ''}
                        onChange={(e) => updateSelectedConfig({ url: e.target.value })}
                      />
                    </div>
                  )}
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    <button className="btn-sm" onClick={bringSelectedToFront}>{t('overlay.bringToFront')}</button>
                    <button className="btn-sm" onClick={removeSelected}>{t('common.delete')}</button>
                  </div>
                </>
              )}
            </div>
          </div>

          <div className="settings-card" style={{ marginTop: 20 }}>
            <div className="settings-card-title">{t('overlay.linksTitle')}</div>
            <div className="sfield" style={{ marginBottom: 10 }}>
              <label>{t('overlay.renderUrlLabel')}</label>
              <div className="api-item">
                <span className="api-key">{renderUrl}</span>
                <button className="btn-sm" onClick={() => navigator.clipboard?.writeText(renderUrl)}>
                  {t('common.copy')}
                </button>
              </div>
              <span className="srv-desc" style={{ fontSize: 10 }}>{t('overlay.renderUrlHint')}</span>
            </div>
            <div className="sfield">
              <label>{t('overlay.moderatorUrlLabel')}</label>
              <div className="api-item">
                <span className="api-key">{moderatorUrl}</span>
                <button className="btn-sm" onClick={() => navigator.clipboard?.writeText(moderatorUrl)}>
                  {t('common.copy')}
                </button>
              </div>
              <span className="srv-desc" style={{ fontSize: 10 }}>{t('overlay.moderatorUrlHint')}</span>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
