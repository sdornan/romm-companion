package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procNames are the comm values Steam's own process uses; the native client is
// "steam", the Flatpak wrapper keeps the same name inside its sandbox.
var procNames = map[string]bool{"steam": true, "steamwebhelper": false}

// running scans /proc rather than shelling out, so it works in a minimal
// container and cannot be defeated by a missing pgrep.
func running() (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			// The process exited between the listing and the read.
			continue
		}
		if procNames[strings.TrimSpace(string(comm))] {
			return true, nil
		}
	}
	return false, nil
}
