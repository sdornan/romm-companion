package process

import (
	"errors"
	"os/exec"
)

func running() (bool, error) {
	// pgrep exits 1 when nothing matches, which is an answer rather than a
	// failure; any other exit status is a real error.
	err := exec.Command("pgrep", "-x", "steam_osx").Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}
