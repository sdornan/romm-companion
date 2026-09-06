package notify

import (
	"context"
	"os/exec"
)

func send(ctx context.Context, title, body string) error {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		return err
	}
	return exec.CommandContext(
		ctx, path, "--app-name=RomM Companion", "--icon=applications-games", title, body,
	).Run()
}
