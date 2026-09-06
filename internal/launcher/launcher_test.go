package launcher

import (
	"context"
	"runtime"
	"testing"

	"github.com/sdornan/romm-companion/internal/emulator"
)

func TestRunSubstitutesRomAndMeasures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	res := emulator.Resolution{Kind: "template", Command: []string{"/bin/sh", "-c", `test "$0" = "/tmp/game.sfc"`, "%ROM%"}}
	s := Run(context.Background(), res, "/tmp/game.sfc")
	if s.Err != nil {
		t.Fatalf("expected %%ROM%% to be substituted: %v", s.Err)
	}
	if s.Duration() < 0 {
		t.Fatal("negative duration")
	}
}

func TestRunRejectsWebPlayer(t *testing.T) {
	s := Run(context.Background(), emulator.Resolution{Kind: "web_player"}, "x")
	if s.Err == nil {
		t.Fatal("web player has no local command")
	}
}
