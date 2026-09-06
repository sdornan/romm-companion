// Package launcher runs an emulator for one ROM and measures the session.
package launcher

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sdornan/romm-companion/internal/emulator"
)

// Session is the outcome of one launch, shaped for RomM's play session ingest.
type Session struct {
	Start time.Time
	End   time.Time
	Err   error
}

// Duration is the wall-clock time the emulator ran.
func (s Session) Duration() time.Duration { return s.End.Sub(s.Start) }

// Run substitutes the ROM path into the resolution's command, starts it with
// the emulator's stdio attached, and waits for exit. The returned session is
// valid even when the emulator failed, so a short crash is still recorded.
func Run(ctx context.Context, res emulator.Resolution, romPath string) Session {
	s := Session{Start: time.Now()}
	if res.Kind == "web_player" || len(res.Command) == 0 {
		s.End = time.Now()
		s.Err = errors.New("launcher: resolution has no local command")
		return s
	}
	sub := substituter(romPath)
	argv := make([]string, len(res.Command))
	for i, a := range res.Command {
		argv[i] = sub(a)
	}
	// A template may name a multi-word executable such as "flatpak run <id>".
	if head := strings.Fields(argv[0]); len(head) > 1 {
		argv = append(head, argv[1:]...)
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = sub(res.WorkDir)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	s.Err = cmd.Run()
	s.End = time.Now()
	return s
}

// substituter returns the per-ROM placeholder replacement for one launch.
// ES-DE's standalone commands use all three; a user template usually only
// uses %ROM%.
func substituter(romPath string) func(string) string {
	base := filepath.Base(romPath)
	replacer := strings.NewReplacer(
		"%ROM%", romPath,
		"%GAMEDIR%", filepath.Dir(romPath),
		"%BASENAME%", strings.TrimSuffix(base, filepath.Ext(base)),
	)
	return replacer.Replace
}
