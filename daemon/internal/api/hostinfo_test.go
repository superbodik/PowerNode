package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMeminfo(t *testing.T) {
	content := `MemTotal:        8144936 kB
MemFree:          412300 kB
MemAvailable:    3221388 kB
Buffers:          102412 kB
Cached:          2841200 kB
`
	path := filepath.Join(t.TempDir(), "meminfo")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	totalMB, availableMB := parseMeminfo(path)
	if totalMB != 7954 {
		t.Errorf("totalMB = %d, want 7954 (8144936 kB / 1024)", totalMB)
	}
	if availableMB != 3145 {
		t.Errorf("availableMB = %d, want 3145 (3221388 kB / 1024)", availableMB)
	}
}

func TestParseMeminfoMissingFile(t *testing.T) {
	totalMB, availableMB := parseMeminfo(filepath.Join(t.TempDir(), "does-not-exist"))
	if totalMB != 0 || availableMB != 0 {
		t.Errorf("expected zeros for a missing file, got total=%d available=%d", totalMB, availableMB)
	}
}
