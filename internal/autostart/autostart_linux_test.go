package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnitFileRunsTheCompanionAtLogin(t *testing.T) {
	unit := UnitFile("/opt/romm/romm-companion")
	for _, want := range []string{
		"ExecStart=/opt/romm/romm-companion run",
		"WantedBy=default.target",
		"Restart=on-failure",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit is missing %q:\n%s", want, unit)
		}
	}
}

func TestInstallWritesTheUnitAndUninstallRemovesIt(t *testing.T) {
	// XDG_CONFIG_HOME redirects os.UserConfigDir, so this never touches the
	// developer's own systemd directory.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	status, err := Install("/usr/local/bin/romm-companion")
	// systemctl is absent in CI; the unit must still be written.
	if err != nil && !strings.Contains(err.Error(), "could not be enabled") {
		t.Fatal(err)
	}
	unit := filepath.Join(dir, "systemd", "user", "romm-companion.service")
	if status.Path != unit {
		t.Fatalf("path = %q, want %q", status.Path, unit)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("unit not written: %v", err)
	}

	if got, err := Current(); err != nil || !got.Installed {
		t.Fatalf("Current() = %+v, %v", got, err)
	}
	if err := Uninstall(); err != nil {
		t.Fatal(err)
	}
	if got, err := Current(); err != nil || got.Installed {
		t.Fatalf("after uninstall Current() = %+v, %v", got, err)
	}
	if err := Uninstall(); err != nil {
		t.Fatalf("uninstalling twice should succeed: %v", err)
	}
}
