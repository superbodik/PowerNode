import type { ServerStatus } from '../types';

const LABELS: Record<ServerStatus, string> = {
  running: 'Online',
  offline: 'Offline',
  starting: 'Starting',
  stopping: 'Stopping',
  installing: 'Installing',
  install_failed: 'Install failed',
  suspended: 'Suspended',
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
      {LABELS[status]}
    </div>
  );
}
