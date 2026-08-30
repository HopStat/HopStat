package updater

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	osStat      = os.Stat
	osReadFile  = os.ReadFile
	runtimeGOOS = runtime.GOOS
	osWriteFile = os.WriteFile
)

// SelfUpdateSupported reports whether the current process can replace its own binary.
func SelfUpdateSupported() (bool, string) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LG_UPDATE_ENABLED")), "false") {
		return false, "disabled via LG_UPDATE_ENABLED=false"
	}
	if runtimeGOOS != "linux" {
		return false, "self-update is only supported on Linux"
	}
	if _, err := osStat("/.dockerenv"); err == nil {
		return false, "docker deployment — pull a newer image instead"
	}
	if data, err := osReadFile("/proc/1/cgroup"); err == nil {
		cgroup := string(data)
		if strings.Contains(cgroup, "docker") || strings.Contains(cgroup, "containerd") || strings.Contains(cgroup, "kubepods") {
			return false, "container deployment — pull a newer image instead"
		}
	}

	execPath, err := osExecutable()
	if err != nil {
		return false, "cannot resolve executable path"
	}
	execPath, err = evalSymlinks(execPath)
	if err != nil {
		return false, "cannot resolve executable path"
	}

	dir := filepath.Dir(execPath)
	testFile := filepath.Join(dir, ".hopstat-write-test")
	if err := osWriteFile(testFile, []byte("x"), 0644); err != nil {
		return false, "binary directory is not writable"
	}
	_ = os.Remove(testFile)
	return true, ""
}
