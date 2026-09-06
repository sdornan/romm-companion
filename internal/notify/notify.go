// Package notify shows a desktop notification.
//
// The companion runs in the background with no window of its own, so this is
// how it says a change is waiting on a Steam restart. Every call is
// best-effort: a headless machine, a missing notifier, or a locked session
// must never fail a reconcile pass.
package notify

import (
	"context"
	"strings"
)

// Send shows one notification. The error is returned for logging only; callers
// are expected to carry on regardless.
func Send(ctx context.Context, title, body string) error {
	return send(ctx, title, body)
}

// Both macOS and Windows build their notification by interpolating text into a
// script, so a game name carrying a quote must not break out of the string
// literal around it. These live here, rather than beside the platform that
// uses each one, so they compile and are tested everywhere.

// escapeAppleScript escapes a value for an AppleScript double-quoted string.
func escapeAppleScript(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// escapePowerShell escapes a value for a PowerShell single-quoted string,
// where a literal quote is written twice.
func escapePowerShell(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
