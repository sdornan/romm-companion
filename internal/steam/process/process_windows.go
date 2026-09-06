package process

import (
	"os/exec"
	"strings"
)

func running() (bool, error) {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq steam.exe", "/NH").Output()
	if err != nil {
		return false, err
	}
	// tasklist prints an "INFO: No tasks..." banner rather than failing when
	// the filter matches nothing.
	return strings.Contains(strings.ToLower(string(out)), "steam.exe"), nil
}
