package emulator

import (
	"errors"
	"testing"

	"github.com/sdornan/romm-companion/internal/emulator/esde"
)

func newTestResolver(retroarch string) *Resolver {
	r := &Resolver{
		Templates: map[string]string{"psx": `duckstation -batch "%ROM%"`},
		Cores:     map[string][]string{"snes": {"snes9x", "bsnes"}, "psx": {"pcsx_rearmed"}},
		LookPath: func(name string) (string, error) {
			if retroarch != "" && (name == "retroarch" || name == "retroarch.exe") {
				return retroarch, nil
			}
			return "", errors.New("not found")
		},
	}
	r.RetroArch = r.findRetroArch()
	return r
}

func TestResolveOrder(t *testing.T) {
	r := newTestResolver("/usr/bin/retroarch")

	if got := r.Resolve("psx"); got.Kind != "template" || len(got.Command) != 3 || got.Command[2] != "%ROM%" {
		t.Fatalf("template should win and keep quoted %%ROM%%: %+v", got)
	}
	if got := r.Resolve("snes"); got.Descriptor != "retroarch:snes9x" || got.Command[0] != "/usr/bin/retroarch" {
		t.Fatalf("retroarch fallback: %+v", got)
	}
	if got := r.Resolve("switch"); got.Supported() {
		t.Fatalf("unknown platform should be unsupported: %+v", got)
	}

	r.WebPlayerURL = "https://romm.example"
	if got := r.Resolve("switch"); got.Kind != "web_player" {
		t.Fatalf("web player fallback: %+v", got)
	}
}

func TestCapabilitiesReportsNilForUnsupported(t *testing.T) {
	r := newTestResolver("")
	caps := r.Capabilities([]string{"snes", "psx"})
	if caps["snes"] != nil {
		t.Fatal("without RetroArch, snes should be nil")
	}
	if caps["psx"] == nil || *caps["psx"] != "template" {
		t.Fatalf("psx template: %v", caps["psx"])
	}
}

func TestSplitTemplate(t *testing.T) {
	got := splitTemplate(`retroarch -L "cores/snes9x libretro.so" %ROM%`)
	want := []string{"retroarch", "-L", "cores/snes9x libretro.so", "%ROM%"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// esdeResolver builds a resolver with a two-alternative ES-DE table where only
// the second emulator is installed.
func esdeResolver(t *testing.T) *Resolver {
	t.Helper()
	r := newTestResolver("/usr/bin/retroarch")
	r.RomRoot = "/games"
	r.ESDE = &esde.Data{
		Systems: map[string][]esde.Alternative{
			"snes": {
				{Label: "bsnes (Standalone)", Emulator: "BSNES", Args: []string{"%ROM%"}},
				{Label: "Mesen (Standalone)", Emulator: "MESEN", Args: []string{"--fullscreen", "%ROM%"}},
			},
			"arcade": {
				{
					Label:    "MAME (Standalone)",
					Emulator: "MAME",
					Args:     []string{"-rompath", "%GAMEDIR%;%ROMPATH%/arcade", "%BASENAME%"},
					StartDir: "%EMUDIR%",
				},
			},
		},
		Emulators: map[string]esde.FindRule{
			"BSNES": {SystemPath: []string{"bsnes"}},
			"MESEN": {SystemPath: []string{"mesen"}},
			"MAME":  {SystemPath: []string{"mame"}},
		},
	}
	r.Finder = &esde.Finder{
		LookPath: func(name string) (string, error) {
			switch name {
			case "mesen":
				return "/opt/mesen/mesen", nil
			case "mame":
				return "/usr/games/mame", nil
			}
			return "", errors.New("not found")
		},
		Glob: func(string) ([]string, error) { return nil, nil },
	}
	return r
}

func TestResolveTakesTheFirstInstalledStandalone(t *testing.T) {
	r := esdeResolver(t)

	got := r.Resolve("snes")
	if got.Kind != "standalone" || got.Descriptor != "standalone:mesen" {
		t.Fatalf("want the installed alternative, got %+v", got)
	}
	if got.Command[0] != "/opt/mesen/mesen" || got.Command[2] != "%ROM%" {
		t.Fatalf("command: %+v", got.Command)
	}
}

func TestResolveSubstitutesTheKnownPlaceholders(t *testing.T) {
	got := esdeResolver(t).Resolve("arcade")
	if got.Command[2] != "%GAMEDIR%;/games/arcade" {
		t.Errorf("%%ROMPATH%% not substituted, %%GAMEDIR%% not left alone: %q", got.Command[2])
	}
	if got.WorkDir != "/usr/games" {
		t.Errorf("start directory: %q", got.WorkDir)
	}
}

func TestStandaloneOutranksRetroArch(t *testing.T) {
	r := esdeResolver(t)
	if got := r.Resolve("snes"); got.Kind != "standalone" {
		t.Fatalf("standalone should win over a RetroArch core: %+v", got)
	}
	// With nothing installed, RetroArch and RomM's core map take over.
	r.Finder = &esde.Finder{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Glob:     func(string) ([]string, error) { return nil, nil },
	}
	if got := r.Resolve("snes"); got.Descriptor != "retroarch:snes9x" {
		t.Fatalf("fallback to RetroArch: %+v", got)
	}
}

func TestTemplateOutranksStandalone(t *testing.T) {
	r := esdeResolver(t)
	r.Templates["snes"] = "myemu %ROM%"
	if got := r.Resolve("snes"); got.Kind != "template" {
		t.Fatalf("the user's template comes first: %+v", got)
	}
}
