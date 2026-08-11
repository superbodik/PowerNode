package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "hello.txt", strings.NewReader("hello world"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("content = %q, want %q", got, "hello world")
	}
}

func TestWriteOverwritesExistingContent(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "hello.txt", strings.NewReader("first"), 0); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := Write(dir, "hello.txt", strings.NewReader("second, and longer"), 0); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "second, and longer" {
		t.Fatalf("content = %q, want %q", got, "second, and longer")
	}
}

func TestWriteLeavesReadablePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("NTFS has no POSIX permission bits; this daemon only ships for Linux")
	}
	dir := t.TempDir()
	if err := Write(dir, "hello.txt", strings.NewReader("x"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestWriteDoesNotLeaveTempFilesBehind(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "hello.txt", strings.NewReader("x"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "hello.txt" {
		t.Fatalf("directory contents = %v, want only hello.txt", entries)
	}
}

func TestWriteConflictOnStaleMtime(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "hello.txt", strings.NewReader("first"), 0); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	staleMtime := info.ModTime().Unix() - 1000
	err = Write(dir, "hello.txt", strings.NewReader("second"), staleMtime)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if string(got) != "first" {
		t.Fatalf("content changed despite conflict: %q", got)
	}
}

type errReader struct{ afterBytes int }

func (r *errReader) Read(p []byte) (int, error) {
	if r.afterBytes <= 0 {
		return 0, errors.New("simulated read failure")
	}
	n := len(p)
	if n > r.afterBytes {
		n = r.afterBytes
	}
	for i := range p[:n] {
		p[i] = 'x'
	}
	r.afterBytes -= n
	return n, nil
}

func TestWriteLeavesOriginalUntouchedOnInterruptedUpload(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "hello.txt", strings.NewReader("original content"), 0); err != nil {
		t.Fatalf("first Write: %v", err)
	}

	err := Write(dir, "hello.txt", &errReader{afterBytes: 4}, 0)
	if err == nil {
		t.Fatal("expected an error from the interrupted upload, got nil")
	}

	got, readErr := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(got) != "original content" {
		t.Fatalf("original file was modified by a failed write: %q", got)
	}

	entries, readDirErr := os.ReadDir(dir)
	if readDirErr != nil {
		t.Fatalf("ReadDir: %v", readDirErr)
	}
	if len(entries) != 1 {
		t.Fatalf("leftover temp file(s) after failed write: %v", entries)
	}
}

func TestWriteRejectsRootDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "/", strings.NewReader("x"), 0); err == nil {
		t.Fatal("expected an error writing to the root directory")
	}
}

// SafeJoin doesn't reject a ".." in the requested path with an error — it
// jails it the same way a chroot would, by cleaning the path against a
// synthetic root before joining it to baseDir. So "../escape.txt" doesn't
// fail, it silently resolves to baseDir/escape.txt. What actually matters is
// that the write can never land outside baseDir no matter how many ".."
// segments are thrown at it.
func TestWriteJailsPathEscapeInsideBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "../../../escape.txt", strings.NewReader("x"), 0); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatalf("expected the write jailed into the base dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("escape.txt should not have been created outside the base dir")
	}
}

var _ io.Reader = (*errReader)(nil)
