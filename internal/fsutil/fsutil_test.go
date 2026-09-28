package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAtomicWriteCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")

	if err := AtomicWrite(path, []byte("one")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := AtomicWrite(path, []byte("two")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "two" {
		t.Fatalf("expected %q, got %q", "two", got)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected temp files to be cleaned up, got %d entries", len(entries))
	}
}

func TestAtomicWritePreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()

	newPath := filepath.Join(dir, "new.txt")
	if err := AtomicWrite(newPath, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(newPath); info.Mode().Perm() != defaultMode {
		t.Fatalf("expected new file mode %v, got %v", defaultMode, info.Mode().Perm())
	}

	existingPath := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(existingPath, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existingPath, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(existingPath, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(existingPath); info.Mode().Perm() != 0o640 {
		t.Fatalf("expected preserved mode 0640, got %v", info.Mode().Perm())
	}
}
