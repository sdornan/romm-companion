package autostart

import (
	"errors"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

// runKey is the per-user key Windows reads at sign-in. Per-user rather than
// the machine-wide equivalent: the companion is bound to one person's RomM
// token and one person's Steam library.
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// RunValue is the command Windows runs at login. The exe path is quoted so a
// path with spaces (the usual case under Program Files) still parses.
func RunValue(exePath string) string {
	return strconv.Quote(exePath) + " run"
}

func install(exePath string) (Status, error) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return Status{}, err
	}
	defer func() { _ = key.Close() }()
	if err := key.SetStringValue(Name, RunValue(exePath)); err != nil {
		return Status{}, err
	}
	return Status{Installed: true, Path: `HKCU\` + runKey + `\` + Name}, nil
}

func uninstall() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = key.Close() }()
	if err := key.DeleteValue(Name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func current() (Status, error) {
	path := `HKCU\` + runKey + `\` + Name
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return Status{Path: path}, nil
		}
		return Status{}, err
	}
	defer func() { _ = key.Close() }()
	if _, _, err := key.GetStringValue(Name); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return Status{Path: path}, nil
		}
		return Status{}, err
	}
	return Status{Installed: true, Path: path}, nil
}
