package oswrapper

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetEnv(t *testing.T) {
	t.Setenv("OSWRAPPER_TEST_SET", "value")
	t.Setenv("OSWRAPPER_TEST_BLANK", "   ")
	t.Setenv("OSWRAPPER_TEST_EMPTY", "")

	osw := New()

	if got, err := osw.GetEnv("OSWRAPPER_TEST_SET"); err != nil || got != "value" {
		t.Fatalf("got %q err=%v", got, err)
	}
	for _, key := range []string{"OSWRAPPER_TEST_BLANK", "OSWRAPPER_TEST_EMPTY", "OSWRAPPER_TEST_MISSING"} {
		got, err := osw.GetEnv(key)
		if err == nil || got != "" {
			t.Fatalf("%s: got %q err=%v want error", key, got, err)
		}
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error must name the key: %v", err)
		}
	}
}

func TestReadFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	osw := New()

	if got, err := osw.ReadFile(path); err != nil || got != "hello\n" {
		t.Fatalf("got %q err=%v", got, err)
	}

	_, err := osw.ReadFile(filepath.Join(dir, "missing.txt"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file must wrap fs.ErrNotExist: %v", err)
	}
}

func TestReadFileResolvesRelativePath(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp(".", "oswrapper-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "rel.txt")
	if err := os.WriteFile(path, []byte("rel"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got, err := New().ReadFile(path); err != nil || got != "rel" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
