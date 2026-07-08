package files

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrConflict = errors.New("file changed on disk since it was last read")

type Entry struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"is_directory"`
	SizeBytes   int64  `json:"size_bytes"`
	ModifiedAt  int64  `json:"modified_at"`
	Mode        string `json:"mode"`
}

func SafeJoin(baseDir, requestedPath string) (string, error) {
	cleaned := filepath.Clean("/" + requestedPath)
	full := filepath.Join(baseDir, cleaned)
	rel, err := filepath.Rel(baseDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes server directory")
	}
	return full, nil
}

func List(baseDir, requestedPath string) ([]Entry, error) {
	dir, err := SafeJoin(baseDir, requestedPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	result := make([]Entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, Entry{
			Name:        e.Name(),
			IsDirectory: e.IsDir(),
			SizeBytes:   info.Size(),
			ModifiedAt:  info.ModTime().Unix(),
			Mode:        info.Mode().String(),
		})
	}
	return result, nil
}

func Read(baseDir, requestedPath string) (*os.File, error) {
	full, err := SafeJoin(baseDir, requestedPath)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

func Write(baseDir, requestedPath string, r io.Reader, expectedMtime int64) error {
	full, err := SafeJoin(baseDir, requestedPath)
	if err != nil {
		return err
	}
	if full == filepath.Clean(baseDir) {
		return fmt.Errorf("cannot write to the server root directory itself")
	}
	if expectedMtime > 0 {
		if info, statErr := os.Stat(full); statErr == nil && info.ModTime().Unix() != expectedMtime {
			return ErrConflict
		}
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func Delete(baseDir, requestedPath string) error {
	full, err := SafeJoin(baseDir, requestedPath)
	if err != nil {
		return err
	}
	if full == filepath.Clean(baseDir) {
		return fmt.Errorf("cannot delete the server root directory itself")
	}
	return os.RemoveAll(full)
}

func Mkdir(baseDir, requestedPath string) error {
	full, err := SafeJoin(baseDir, requestedPath)
	if err != nil {
		return err
	}
	return os.MkdirAll(full, 0755)
}

func Rename(baseDir, fromPath, toPath string) error {
	from, err := SafeJoin(baseDir, fromPath)
	if err != nil {
		return err
	}
	to, err := SafeJoin(baseDir, toPath)
	if err != nil {
		return err
	}
	if from == filepath.Clean(baseDir) {
		return fmt.Errorf("cannot rename the server root directory itself")
	}
	return os.Rename(from, to)
}
