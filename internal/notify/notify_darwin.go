package notify

import (
	"context"
	"os/exec"
)

func send(ctx context.Context, title, body string) error {
	// AppleScript string literals take double quotes and backslashes literally,
	// so both have to be escaped or the script fails to parse.
	script := `display notification "` + escapeAppleScript(body) +
		`" with title "` + escapeAppleScript(title) + `"`
	return exec.CommandContext(ctx, "osascript", "-e", script).Run()
}
