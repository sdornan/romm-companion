package autostart

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// PlistFile renders the launchd agent. RunAtLoad starts it at login and
// KeepAlive brings it back if it dies.
func PlistFile(exePath string) string {
	// The path goes through the XML encoder rather than a format string: a
	// name with an ampersand in it would otherwise produce an invalid plist.
	escaped := func(s string) string {
		var buf []byte
		w := &sliceWriter{&buf}
		_ = xml.EscapeText(w, []byte(s))
		return string(buf)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
</dict>
</plist>
`, Label, escaped(exePath))
}

type sliceWriter struct{ buf *[]byte }

func (w *sliceWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

func install(exePath string) (Status, error) {
	path, err := plistPath()
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Status{}, err
	}
	if err := os.WriteFile(path, []byte(PlistFile(exePath)), 0o644); err != nil {
		return Status{}, err
	}
	// Reload so an existing agent picks up the new path.
	_ = exec.Command("launchctl", "unload", path).Run()
	if out, err := exec.Command("launchctl", "load", path).CombinedOutput(); err != nil {
		return Status{Installed: true, Path: path},
			fmt.Errorf("agent written to %s but could not be loaded: %s", path, out)
	}
	return Status{Installed: true, Path: path}, nil
}

func uninstall() error {
	path, err := plistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func current() (Status, error) {
	path, err := plistPath()
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
