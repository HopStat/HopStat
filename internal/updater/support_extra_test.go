package updater

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSelfUpdateSupportedNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-linux branch")
	}
	t.Setenv("LG_UPDATE_ENABLED", "")
	supported, reason := SelfUpdateSupported()
	if supported {
		t.Fatal("expected unsupported on non-linux")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
}

func TestSelfUpdateSupportedDockerEnv(t *testing.T) {
	origStat := osStat
	origGOOS := runtimeGOOS
	t.Cleanup(func() {
		osStat = origStat
		runtimeGOOS = origGOOS
	})
	runtimeGOOS = "linux"
	osStat = func(name string) (os.FileInfo, error) {
		if name == "/.dockerenv" {
			return nil, nil
		}
		return origStat(name)
	}
	t.Setenv("LG_UPDATE_ENABLED", "")
	supported, reason := SelfUpdateSupported()
	if supported {
		t.Fatal("expected docker to disable self update")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
}

func TestSelfUpdateSupportedContainerCgroup(t *testing.T) {
	origReadFile := osReadFile
	origGOOS := runtimeGOOS
	t.Cleanup(func() {
		osReadFile = origReadFile
		runtimeGOOS = origGOOS
	})
	runtimeGOOS = "linux"
	osReadFile = func(name string) ([]byte, error) {
		if name == "/proc/1/cgroup" {
			return []byte("0::/docker/abc"), nil
		}
		return origReadFile(name)
	}
	t.Setenv("LG_UPDATE_ENABLED", "")
	supported, reason := SelfUpdateSupported()
	if supported {
		t.Fatal("expected container cgroup to disable self update")
	}
	if reason == "" {
		t.Fatal("expected reason")
	}
}

func TestSelfUpdateSupportedContainerdCgroup(t *testing.T) {
	origReadFile := osReadFile
	origGOOS := runtimeGOOS
	t.Cleanup(func() {
		osReadFile = origReadFile
		runtimeGOOS = origGOOS
	})
	runtimeGOOS = "linux"
	osReadFile = func(name string) ([]byte, error) {
		if name == "/proc/1/cgroup" {
			return []byte("0::/containerd/abc"), nil
		}
		return origReadFile(name)
	}
	t.Setenv("LG_UPDATE_ENABLED", "")
	supported, _ := SelfUpdateSupported()
	if supported {
		t.Fatal("expected containerd cgroup to disable self update")
	}
}

func TestSelfUpdateSupportedKubeCgroup(t *testing.T) {
	origReadFile := osReadFile
	origGOOS := runtimeGOOS
	t.Cleanup(func() {
		osReadFile = origReadFile
		runtimeGOOS = origGOOS
	})
	runtimeGOOS = "linux"
	osReadFile = func(name string) ([]byte, error) {
		if name == "/proc/1/cgroup" {
			return []byte("0::/kubepods/abc"), nil
		}
		return origReadFile(name)
	}
	t.Setenv("LG_UPDATE_ENABLED", "")
	supported, _ := SelfUpdateSupported()
	if supported {
		t.Fatal("expected kube cgroup to disable self update")
	}
}

func TestSelfUpdateSupportedLinuxBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only writable directory test")
	}
	t.Setenv("LG_UPDATE_ENABLED", "")
	supported, reason := SelfUpdateSupported()
	if strings.Contains(reason, "docker") || strings.Contains(reason, "container") {
		t.Skip(reason)
	}
	if !supported {
		t.Fatalf("expected supported in writable test environment, reason=%q", reason)
	}
}

func TestSelfUpdateSupportedLinuxWritableWithHook(t *testing.T) {
	origGOOS := runtimeGOOS
	origExecutable := osExecutable
	origEval := evalSymlinks
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		osExecutable = origExecutable
		evalSymlinks = origEval
	})
	runtimeGOOS = "linux"
	dir := t.TempDir()
	binPath := filepath.Join(dir, "hopstat")
	osExecutable = func() (string, error) { return binPath, nil }
	evalSymlinks = func(path string) (string, error) { return path, nil }
	t.Setenv("LG_UPDATE_ENABLED", "")

	supported, reason := SelfUpdateSupported()
	if !supported {
		t.Fatalf("expected supported with writable temp dir, reason=%q", reason)
	}
}
