import { useState } from 'react';
import { t } from '../i18n';

interface Props {
  title: string;
  children: React.ReactNode;
}

/** A collapsed-by-default "how this works" panel, dropped at the top of a
 * server tab. Collapsed so it doesn't add clutter for people who already
 * know the feature, but one click away for people who don't. */
export function GuidePanel({ title, children }: Props) {
  const [open, setOpen] = useState(false);
  return (
    <div className={`guide-panel ${open ? 'open' : ''}`}>
      <button type="button" className="guide-toggle" onClick={() => setOpen((v) => !v)}>
        <span className="guide-icon">{open ? '▾' : '▸'}</span>
        <span>{title}</span>
        {!open && <span className="guide-cta">{t('common.show')}</span>}
      </button>
      {open && <div className="guide-body">{children}</div>}
    </div>
  );
}
