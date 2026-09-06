package emulator

import (
	"errors"
	"testing"
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
