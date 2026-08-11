import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Schedule, ScheduleTask } from '../types';

interface Props {
  uuid: string;
}

type TaskType = 'power' | 'command' | 'backup';

interface TaskFormRow {
  taskType: TaskType;
  action: string;
  command: string;
  backupName: string;
  offsetSeconds: number;
}

function newTaskRow(): TaskFormRow {
  return { taskType: 'power', action: 'restart', command: '', backupName: '', offsetSeconds: 0 };
}

function taskLabel(t: ScheduleTask): string {
  switch (t.action) {
    case 'command':
      return `Command: ${t.payload}`;
    case 'backup':
      return `Backup${t.payload ? ': ' + t.payload : ' (auto-named)'}`;
    default:
      return t.payload;
  }
}

export function ScheduleManager({ uuid }: Props) {
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const [form, setForm] = useState({
    name: '',
    cron_minute: '0',
    cron_hour: '*',
    cron_day_of_week: '*',
    cron_day_of_month: '*',
    only_when_online: true,
  });
  const [tasks, setTasks] = useState<TaskFormRow[]>([newTaskRow()]);

  function refresh() {
    api
      .listSchedules(uuid)
      .then(setSchedules)
      .catch((err) => setError(err instanceof Error ? err.message : String(err)));
  }

  useEffect(refresh, [uuid]);

  function updateTask(index: number, patch: Partial<TaskFormRow>) {
    setTasks((prev) => prev.map((t, i) => (i === index ? { ...t, ...patch } : t)));
  }

  function addTask() {
    setTasks((prev) => [...prev, newTaskRow()]);
  }

  function removeTask(index: number) {
    setTasks((prev) => (prev.length > 1 ? prev.filter((_, i) => i !== index) : prev));
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await api.createSchedule(uuid, {
        name: form.name,
        cron_minute: form.cron_minute,
        cron_hour: form.cron_hour,
        cron_day_of_week: form.cron_day_of_week,
        cron_day_of_month: form.cron_day_of_month,
        only_when_online: form.only_when_online,
        tasks: tasks.map((t) => ({
          action: t.taskType,
          payload: t.taskType === 'power' ? t.action : t.taskType === 'command' ? t.command : t.backupName,
          time_offset_seconds: t.offsetSeconds,
        })),
      });
      setShowForm(false);
      setForm((f) => ({ ...f, name: '' }));
      setTasks([newTaskRow()]);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleToggle(s: Schedule) {
    try {
      await api.toggleSchedule(uuid, s.id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleDelete(s: Schedule) {
    if (!window.confirm(`Delete schedule "${s.name}"?`)) return;
    try {
      await api.deleteSchedule(uuid, s.id);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <div>
      {error && (
        <div className="login-error show" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      <div style={{ marginBottom: 16 }}>
        <button className="btn-sm primary" onClick={() => setShowForm((f) => !f)}>
          {showForm ? 'Cancel' : '+ New Schedule'}
        </button>
      </div>

      {showForm && (
        <div className="settings-card" style={{ marginBottom: 20 }}>
          <div className="settings-card-title">New schedule</div>
          <form onSubmit={handleCreate}>
            <div className="settings-grid">
              <div className="sfield span2">
                <label htmlFor="sch-name">Name</label>
                <input
                  id="sch-name"
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  placeholder="Nightly restart"
                  required
                />
              </div>
              <div className="sfield">
                <label htmlFor="sch-minute">Minute</label>
                <input
                  id="sch-minute"
                  value={form.cron_minute}
                  onChange={(e) => setForm((f) => ({ ...f, cron_minute: e.target.value }))}
                  placeholder="* or 0-59"
                />
              </div>
              <div className="sfield">
                <label htmlFor="sch-hour">Hour</label>
                <input
                  id="sch-hour"
                  value={form.cron_hour}
                  onChange={(e) => setForm((f) => ({ ...f, cron_hour: e.target.value }))}
                  placeholder="* or 0-23"
                />
              </div>
              <div className="sfield">
                <label htmlFor="sch-dom">Day of month</label>
                <input
                  id="sch-dom"
                  value={form.cron_day_of_month}
                  onChange={(e) => setForm((f) => ({ ...f, cron_day_of_month: e.target.value }))}
                  placeholder="* or 1-31"
                />
              </div>
              <div className="sfield">
                <label htmlFor="sch-dow">Day of week</label>
                <input
                  id="sch-dow"
                  value={form.cron_day_of_week}
                  onChange={(e) => setForm((f) => ({ ...f, cron_day_of_week: e.target.value }))}
                  placeholder="* or 0=Sun..6=Sat"
                />
              </div>
            </div>

            <div style={{ marginTop: 16 }}>
              <div className="settings-card-title" style={{ fontSize: 13 }}>
                Steps — run in order, each with its own delay before it starts
              </div>
              {tasks.map((task, i) => (
                <div
                  key={i}
                  className="settings-grid"
                  style={{ marginTop: 10, paddingTop: 10, borderTop: i > 0 ? '1px solid var(--border)' : undefined }}
                >
                  <div className="sfield">
                    <label htmlFor={`sch-task-type-${i}`}>Step {i + 1}</label>
                    <select
                      id={`sch-task-type-${i}`}
                      value={task.taskType}
                      onChange={(e) => updateTask(i, { taskType: e.target.value as TaskType })}
                    >
                      <option value="power">Power action</option>
                      <option value="command">Console command</option>
                      <option value="backup">Backup</option>
                    </select>
                  </div>
                  {task.taskType === 'power' ? (
                    <div className="sfield">
                      <label htmlFor={`sch-action-${i}`}>Action</label>
                      <select
                        id={`sch-action-${i}`}
                        value={task.action}
                        onChange={(e) => updateTask(i, { action: e.target.value })}
                      >
                        <option value="start">Start</option>
                        <option value="stop">Stop</option>
                        <option value="restart">Restart</option>
                        <option value="kill">Kill</option>
                      </select>
                    </div>
                  ) : task.taskType === 'command' ? (
                    <div className="sfield">
                      <label htmlFor={`sch-command-${i}`}>Command</label>
                      <input
                        id={`sch-command-${i}`}
                        value={task.command}
                        onChange={(e) => updateTask(i, { command: e.target.value })}
                        placeholder="say Server restarting soon"
                        required
                      />
                    </div>
                  ) : (
                    <div className="sfield">
                      <label htmlFor={`sch-backup-name-${i}`}>Backup name (optional)</label>
                      <input
                        id={`sch-backup-name-${i}`}
                        value={task.backupName}
                        onChange={(e) => updateTask(i, { backupName: e.target.value })}
                        placeholder="nightly"
                      />
                    </div>
                  )}
                  <div className="sfield">
                    <label htmlFor={`sch-offset-${i}`}>Delay before this step (seconds)</label>
                    <input
                      id={`sch-offset-${i}`}
                      type="number"
                      min={0}
                      value={task.offsetSeconds}
                      onChange={(e) => updateTask(i, { offsetSeconds: Number(e.target.value) })}
                    />
                  </div>
                  <div className="sfield" style={{ justifyContent: 'flex-end', flexDirection: 'row', display: 'flex', alignItems: 'flex-end' }}>
                    <button
                      type="button"
                      className="file-act-btn del"
                      onClick={() => removeTask(i)}
                      disabled={tasks.length === 1}
                    >
                      Remove step
                    </button>
                  </div>
                </div>
              ))}
              <button type="button" className="btn-sm" style={{ marginTop: 12 }} onClick={addTask}>
                + Add step
              </button>
            </div>

            <div className="settings-foot">
              <button
                className="btn-primary"
                type="submit"
                disabled={submitting}
                style={{ width: 'auto', padding: '10px 20px' }}
              >
                {submitting ? 'Creating…' : 'Create'}
              </button>
            </div>
          </form>
        </div>
      )}

      <div className="sch-list">
        {schedules.map((s) => (
          <div className="sch-card" key={s.id}>
            <div className="sch-head">
              <span className="sch-name">{s.name}</span>
              <div className="sch-toggle">
                <div
                  className={`toggle-sw ${s.is_active ? 'on' : ''}`}
                  onClick={() => handleToggle(s)}
                >
                  <div className="toggle-knob" />
                </div>
              </div>
            </div>
            <div className="sch-cron" style={{ display: 'inline-block', marginBottom: 10 }}>
              {s.cron_minute} {s.cron_hour} {s.cron_day_of_month} * {s.cron_day_of_week}
            </div>
            <div className="sch-meta" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 4 }}>
              {s.tasks.length > 0 ? (
                s.tasks.map((t, i) => (
                  <span key={i}>
                    {i + 1}. {taskLabel(t)}
                    {t.time_offset_seconds > 0 ? ` (+${t.time_offset_seconds}s)` : ''}
                  </span>
                ))
              ) : (
                <span>—</span>
              )}
            </div>
            <div className="sch-meta">
              <span>{s.only_when_online ? 'Only when online' : 'Always'}</span>
              <span>
                {s.last_run_at ? `Last run: ${new Date(s.last_run_at).toLocaleString()}` : 'Never run'}
              </span>
              <button className="file-act-btn del" onClick={() => handleDelete(s)}>
                Delete
              </button>
            </div>
          </div>
        ))}
        {schedules.length === 0 && <p className="srv-desc">No schedules yet.</p>}
      </div>
    </div>
  );
}
