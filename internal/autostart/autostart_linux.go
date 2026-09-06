package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// UnitFile renders the systemd user unit. Restart=on-failure rather than
// always: a clean exit means the user stopped it deliberately.
func UnitFile(exePath string) string {
	return fmt.Sprintf(`[Unit]
Description=RomM Companion
Documentation=https://github.com/sdornan/romm-companion
After=network-online.target

[Service]
Type=simple
ExecStart=%s run
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`, exePath)
}

func unitPath() (string, error) {
	home, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "systemd", "user", Name+".service"), nil
}

func install(exePath string) (Status, error) {
	path, err := unitPath()
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Status{}, err
	}
	if err := os.WriteFile(path, []byte(UnitFile(exePath)), 0o644); err != nil {
		return Status{}, err
	}
	// systemd caches unit files, so a rewrite needs a reload to take effect.
	if systemctl, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command(systemctl, "--user", "daemon-reload").Run()
		if out, err := exec.Command(
			systemctl, "--user", "enable", "--now", Name+".service",
		).CombinedOutput(); err != nil {
			return Status{Installed: true, Path: path},
				fmt.Errorf("unit written to %s but could not be enabled: %s", path, out)
		}
	}
	return Status{Installed: true, Path: path}, nil
}

func uninstall() error {
	path, err := unitPath()
	if err != nil {
		return err
	}
	if systemctl, lookErr := exec.LookPath("systemctl"); lookErr == nil {
		_ = exec.Command(systemctl, "--user", "disable", "--now", Name+".service").Run()
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func current() (Status, error) {
	path, err := unitPath()
	if err != nil {
		return Status{}, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return Status{Path: path}, nil
		}
		return Status{}, err
	}
	return Status{Installed: true, Path: path}, nil
}
