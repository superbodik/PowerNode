import { t } from '../i18n';
import type { ServerStatus } from '../types';

const LABEL_KEYS: Record<ServerStatus, Parameters<typeof t>[0]> = {
  running: 'status.running',
  offline: 'status.offline',
  starting: 'status.starting',
  stopping: 'status.stopping',
  installing: 'status.installing',
  install_failed: 'status.install_failed',
  suspended: 'status.suspended',
};

const BADGE_CLASS: Partial<Record<ServerStatus, string>> = {
  running: 'online',
  offline: 'offline',
  starting: 'starting',
  stopping: 'stopping',
};

export function StatusBadge({ status }: { status: ServerStatus }) {
  const variant = BADGE_CLASS[status] ?? 'offline';
  return (
    <div className={`status-badge ${variant}`}>
      <span className="dot" />
      {t(LABEL_KEYS[status])}
    </div>
  );
}
