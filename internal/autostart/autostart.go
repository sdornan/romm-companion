// Package autostart registers the companion to run at login.
//
// The companion has no window, so "start it yourself every boot" is the same
// as "it does not work". Each platform gets its native mechanism rather than a
// cross-platform shim: a systemd user unit, a launchd agent, or a registry Run
// entry. The file each one needs is built by a pure function so its exact
// contents can be tested anywhere.
package autostart

// Label is the reverse-DNS identifier used where a platform wants one.
const Label = "app.romm.companion"

// Name is the service/unit/value name used where a plain name is wanted.
const Name = "romm-companion"

// Status describes whether the companion is registered to start at login.
type Status struct {
	// Installed reports whether the login entry exists.
	Installed bool
	// Path is where that entry lives, for the user to inspect or delete.
	Path string
}

// Install registers exePath to run `romm-companion run` at login, replacing
// any previous registration.
func Install(exePath string) (Status, error) { return install(exePath) }

// Uninstall removes the login entry. Removing one that is not there succeeds.
func Uninstall() error { return uninstall() }

// Current reports whether the login entry exists.
func Current() (Status, error) { return current() }
