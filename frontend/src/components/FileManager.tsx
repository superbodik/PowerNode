import { useEffect, useRef, useState } from 'react';
import { api, FileConflictError } from '../api/client';
import { GuidePanel } from './GuidePanel';
import { t } from '../i18n';
import type { FileEntry } from '../types';

interface Props {
  uuid: string;
}

function joinPath(dir: string, name: string): string {
  return dir.replace(/\/$/, '') + '/' + name;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  return `${(kb / 1024).toFixed(1)} MB`;
}

function formatDate(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString();
}

const NUL = String.fromCharCode(0);
const REPLACEMENT_CHAR = String.fromCharCode(0xfffd);

function looksBinary(content: string): boolean {
  return content.indexOf(NUL) !== -1 || content.indexOf(REPLACEMENT_CHAR) !== -1;
}

export function FileManager({ uuid }: Props) {
  const [path, setPath] = useState('/');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editingFile, setEditingFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState('');
  const [editingMtime, setEditingMtime] = useState(0);
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [creatingFolder, setCreatingFolder] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [renamingEntry, setRenamingEntry] = useState<FileEntry | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  function refresh() {
    setLoading(true);
    setError(null);
    api
      .listFiles(uuid, path)
      .then(setEntries)
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  }

  useEffect(refresh, [uuid, path]);

  const segments = path.split('/').filter(Boolean);

  function goToSegment(index: number) {
    setPath('/' + segments.slice(0, index + 1).join('/'));
  }

  async function openEntry(entry: FileEntry) {
    if (entry.is_directory) {
      setPath(joinPath(path, entry.name));
      return;
    }
    const target = joinPath(path, entry.name);
    try {
      const { text, mtime } = await api.readFile(uuid, target);
      if (looksBinary(text)) {
        setError(t('files.looksBinary', { name: entry.name }));
        return;
      }
      setEditingFile(target);
      setFileContent(text);
      setEditingMtime(mtime);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleSave() {
    if (!editingFile) return;
    setSaving(true);
    try {
      await api.writeFile(uuid, editingFile, fileContent, editingMtime);
      setEditingFile(null);
      refresh();
    } catch (err) {
      if (err instanceof FileConflictError) {
        setError(t('files.conflict', { name: editingFile.split('/').pop() ?? editingFile }));
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(entry: FileEntry, e: React.MouseEvent) {
    e.stopPropagation();
    if (!window.confirm(t('files.confirmDelete', { name: entry.name }))) return;
    try {
      await api.deleteFile(uuid, joinPath(path, entry.name));
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  function startRename(entry: FileEntry, e: React.MouseEvent) {
    e.stopPropagation();
    setRenamingEntry(entry);
    setRenameValue(entry.name);
  }

  async function submitRename(e: React.FormEvent) {
    e.preventDefault();
    if (!renamingEntry) return;
    const newName = renameValue.trim();
    if (!newName || newName === renamingEntry.name) {
      setRenamingEntry(null);
      return;
    }
    try {
      await api.renameFile(uuid, joinPath(path, renamingEntry.name), joinPath(path, newName));
      setRenamingEntry(null);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleDownload(entry: FileEntry, e: React.MouseEvent) {
    e.stopPropagation();
    try {
      const blob = await api.downloadFile(uuid, joinPath(path, entry.name));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = entry.name;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function handleUploadChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      await api.uploadFile(uuid, joinPath(path, file.name), file);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  }

  async function submitNewFolder(e: React.FormEvent) {
    e.preventDefault();
    const name = newFolderName.trim();
    if (!name) {
      setCreatingFolder(false);
      return;
    }
    try {
      await api.createDirectory(uuid, joinPath(path, name));
      setCreatingFolder(false);
      setNewFolderName('');
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  if (editingFile) {
    return (
      <div>
        <div className="files-toolbar">
          <div className="files-path">{editingFile}</div>
          <button className="btn-sm primary" onClick={handleSave} disabled={saving}>
            {saving ? t('common.saving') : t('common.save')}
          </button>
          <button className="btn-sm" onClick={() => setEditingFile(null)}>
            {t('common.cancel')}
          </button>
        </div>
        <textarea
          value={fileContent}
          onChange={(e) => setFileContent(e.target.value)}
          spellCheck={false}
          style={{
            width: '100%',
            minHeight: 380,
            background: '#070508',
            color: 'var(--text)',
            border: '1px solid var(--border)',
            borderRadius: 12,
            padding: 14,
            fontFamily: 'var(--font-mono)',
            fontSize: 12,
            resize: 'vertical',
          }}
        />
      </div>
    );
  }

  return (
    <div>
      <GuidePanel title={t('guide.files.title')}>
        <p>{t('guide.files.p1')}</p>
        <p>{t('guide.files.p2')}</p>
        <p>{t('guide.files.p3')}</p>
        <p>{t('guide.files.p4')}</p>
      </GuidePanel>
      <div className="files-toolbar">
        <div className="files-path">
          <span className="path-seg" onClick={() => setPath('/')}>
            /
          </span>
          {segments.map((seg, i) => (
            <span key={i}>
              <span className="path-sep"> / </span>
              <span className="path-seg" onClick={() => goToSegment(i)}>
                {seg}
              </span>
            </span>
          ))}
        </div>
        <input
          type="file"
          ref={fileInputRef}
          style={{ display: 'none' }}
          onChange={handleUploadChange}
        />
        <button className="btn-sm" onClick={() => fileInputRef.current?.click()} disabled={uploading}>
          {uploading ? t('files.uploading') : t('files.upload')}
        </button>
        <button
          className="btn-sm primary"
          onClick={() => {
            setCreatingFolder((v) => !v);
            setNewFolderName('');
          }}
        >
          {t('files.addFolder')}
        </button>
      </div>

      {creatingFolder && (
        <form className="files-inline-form" onSubmit={submitNewFolder}>
          <input
            autoFocus
            value={newFolderName}
            onChange={(e) => setNewFolderName(e.target.value)}
            placeholder={t('files.folderName')}
          />
          <button className="btn-sm primary" type="submit">
            {t('files.create')}
          </button>
          <button className="btn-sm" type="button" onClick={() => setCreatingFolder(false)}>
            {t('common.cancel')}
          </button>
        </form>
      )}

      {renamingEntry && (
        <form className="files-inline-form" onSubmit={submitRename}>
          <span className="srv-desc">{t('files.renamePrefix', { name: renamingEntry.name })}</span>
          <input
            autoFocus
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
          />
          <button className="btn-sm primary" type="submit">
            {t('common.save')}
          </button>
          <button className="btn-sm" type="button" onClick={() => setRenamingEntry(null)}>
            {t('common.cancel')}
          </button>
        </form>
      )}

      {error && (
        <div className="login-error show" style={{ marginBottom: 12 }}>
          {error}
        </div>
      )}

      <div className="files-table">
        <div className="files-table-head">
          <span>{t('files.colName')}</span>
          <span>{t('files.colSize')}</span>
          <span>{t('files.colModified')}</span>
          <span>{t('files.colActions')}</span>
        </div>
        {loading ? (
          <p className="srv-desc" style={{ padding: 16 }}>
            {t('common.loading')}
          </p>
        ) : (
          entries.map((entry) => (
            <div className="file-row" key={entry.name} onClick={() => openEntry(entry)}>
              <div className="file-name">
                <span className="file-icon">{entry.is_directory ? '📁' : '📄'}</span>
                <span>{entry.name}</span>
              </div>
              <span className="file-size">
                {entry.is_directory ? '—' : formatSize(entry.size_bytes)}
              </span>
              <span className="file-modified">{formatDate(entry.modified_at)}</span>
              <div className="file-actions">
                {!entry.is_directory && (
                  <button
                    className="file-act-btn"
                    title={t('files.download')}
                    onClick={(e) => handleDownload(entry, e)}
                  >
                    ⬇
                  </button>
                )}
                <button className="file-act-btn" title={t('files.rename')} onClick={(e) => startRename(entry, e)}>
                  ✎
                </button>
                <button
                  className="file-act-btn del"
                  title={t('files.delete')}
                  onClick={(e) => handleDelete(entry, e)}
                >
                  ✕
                </button>
              </div>
            </div>
          ))
        )}
        {!loading && entries.length === 0 && (
          <p className="srv-desc" style={{ padding: 16 }}>
            {t('files.emptyDirectory')}
          </p>
        )}
      </div>
    </div>
  );
}
