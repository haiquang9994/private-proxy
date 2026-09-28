// Package fsutil holds small filesystem helpers shared across packages.
package fsutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultMode is used when the target file does not exist yet.
const defaultMode fs.FileMode = 0o644

// AtomicWrite replaces path with data by writing a temp file in the same
// directory and renaming it over path, so readers never see a partial file.
// The existing file's permission bits are preserved (defaultMode for a new
// file), since os.CreateTemp would otherwise leave the result at 0600.
func AtomicWrite(path string, data []byte) error {
	mode := defaultMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
