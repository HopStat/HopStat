package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSelfUpdateSupportedExecutableError(t *testing.T) {
	origGOOS := runtimeGOOS
	origExecutable := osExecutable
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
	})
	runtimeGOOS = "linux"
	osExecutable = func() (string, error) {
		return "", errors.New("boom")
	}
	supported, reason := SelfUpdateSupported()
	if supported {
		t.Fatal("expected unsupported")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
}

func TestSelfUpdateSupportedEvalSymlinksError(t *testing.T) {
	origGOOS := runtimeGOOS
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	osExecutable = func() (string, error) { return "/tmp/hopstat", nil }
	evalSymlinks = func(path string) (string, error) {
		return "", errors.New("boom")
	}
	supported, reason := SelfUpdateSupported()
	if supported {
		t.Fatal("expected unsupported")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
}

func TestSelfUpdateSupportedNonWritableDirectory(t *testing.T) {
	origGOOS := runtimeGOOS
	dir := t.TempDir()
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	binPath := filepath.Join(dir, "hopstat")
	osExecutable = func() (string, error) { return binPath, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }

	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	supported, reason := SelfUpdateSupported()
	if supported {
		t.Fatal("expected non-writable directory to disable self update")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
}
