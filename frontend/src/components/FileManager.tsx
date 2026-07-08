import { useEffect, useRef, useState } from 'react';
import { api, FileConflictError } from '../api/client';
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
        setError(`"${entry.name}" looks like a binary file — use Download instead of editing it as text.`);
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
        setError(`"${editingFile.split('/').pop()}" was changed on disk since you opened it — reopen it to see the latest version before saving again.`);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(entry: FileEntry, e: React.MouseEvent) {
    e.stopPropagation();
    if (!window.confirm(`Delete ${entry.name}?`)) return;
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
            {saving ? 'Saving…' : 'Save'}
          </button>
          <button className="btn-sm" onClick={() => setEditingFile(null)}>
            Cancel
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
          {uploading ? 'Uploading…' : 'Upload'}
        </button>
        <button
          className="btn-sm primary"
          onClick={() => {
            setCreatingFolder((v) => !v);
            setNewFolderName('');
          }}
        >
          + Folder
        </button>
      </div>

      {creatingFolder && (
        <form className="files-inline-form" onSubmit={submitNewFolder}>
          <input
            autoFocus
            value={newFolderName}
            onChange={(e) => setNewFolderName(e.target.value)}
            placeholder="Folder name"
          />
          <button className="btn-sm primary" type="submit">
            Create
          </button>
          <button className="btn-sm" type="button" onClick={() => setCreatingFolder(false)}>
            Cancel
          </button>
        </form>
      )}

      {renamingEntry && (
        <form className="files-inline-form" onSubmit={submitRename}>
          <span className="srv-desc">Rename "{renamingEntry.name}" to</span>
          <input
            autoFocus
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
          />
          <button className="btn-sm primary" type="submit">
            Save
          </button>
          <button className="btn-sm" type="button" onClick={() => setRenamingEntry(null)}>
            Cancel
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
          <span>Name</span>
          <span>Size</span>
          <span>Modified</span>
          <span>Actions</span>
        </div>
        {loading ? (
          <p className="srv-desc" style={{ padding: 16 }}>
            Loading…
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
                    title="Download"
                    onClick={(e) => handleDownload(entry, e)}
                  >
                    ⬇
                  </button>
                )}
                <button className="file-act-btn" title="Rename" onClick={(e) => startRename(entry, e)}>
                  ✎
                </button>
                <button
                  className="file-act-btn del"
                  title="Delete"
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
            Empty directory.
          </p>
        )}
      </div>
    </div>
  );
}
