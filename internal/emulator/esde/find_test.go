package esde

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func notOnPath(string) (string, error) { return "", errors.New("not found") }

// writeExecutable creates a runnable file and returns its path.
func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindPrefersThePathOverStaticEntries(t *testing.T) {
	dir := t.TempDir()
	static := writeExecutable(t, dir, "pcsx2-qt")
	f := &Finder{
		LookPath: func(name string) (string, error) {
			if name == "pcsx2-qt" {
				return "/usr/bin/pcsx2-qt", nil
			}
			return "", errors.New("not found")
		},
		Glob: filepath.Glob,
	}
	rule := FindRule{SystemPath: []string{"pcsx2-qt"}, StaticPath: []string{static}}
	if got := f.Find("PCSX2", rule); got != "/usr/bin/pcsx2-qt" {
		t.Fatalf("want the PATH hit, got %q", got)
	}
}

func TestFindExpandsHomeAndGlobs(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(home, "Applications"), "RetroArch-Linux-x86_64.AppImage")

	f := &Finder{LookPath: notOnPath, Glob: filepath.Glob, Home: home}
	rule := FindRule{StaticPath: []string{"~/Applications/RetroArch-Linux*.AppImage"}}
	got := f.Find("RETROARCH", rule)
	if want := filepath.Join(home, "Applications", "RetroArch-Linux-x86_64.AppImage"); got != want {
		t.Fatalf("glob under ~: got %q want %q", got, want)
	}
}

func TestFindSkipsUnresolvablePlaceholders(t *testing.T) {
	f := &Finder{LookPath: notOnPath, Glob: filepath.Glob}
	rule := FindRule{StaticPath: []string{`%ESPATH%\Emulators\pcsx2-qt.exe`}}
	if got := f.Find("PCSX2", rule); got != "" {
		t.Fatalf("want no match without an app directory, got %q", got)
	}
}

func TestFindIgnoresDirectoriesAndNonExecutables(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ares"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "mednafen")
	if err := os.WriteFile(plain, []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &Finder{LookPath: notOnPath, Glob: filepath.Glob}
	if got := f.Find("ARES", FindRule{StaticPath: []string{filepath.Join(dir, "ares")}}); got != "" {
		t.Errorf("a directory is not an emulator: %q", got)
	}
	if got := f.Find("MEDNAFEN", FindRule{StaticPath: []string{plain}}); got != "" {
		t.Errorf("a non-executable file is not an emulator: %q", got)
	}
}

func TestFindCachesPerEmulator(t *testing.T) {
	calls := 0
	f := &Finder{
		LookPath: func(string) (string, error) {
			calls++
			return "/usr/bin/ares", nil
		},
		Glob: filepath.Glob,
	}
	rule := FindRule{SystemPath: []string{"ares"}}
	for range 3 {
		f.Find("ARES", rule)
	}
	if calls != 1 {
		t.Fatalf("want one lookup for three resolutions, got %d", calls)
	}
}

func TestLoadParsesTheEmbeddedTable(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Systems) < 50 {
		t.Fatalf("only %d platforms in the embedded table", len(d.Systems))
	}
	for slug, alts := range d.Systems {
		for _, a := range alts {
			if a.Emulator == "" || len(a.Args) == 0 {
				t.Fatalf("%s: incomplete alternative %+v", slug, a)
			}
			if _, ok := d.Emulators[a.Emulator]; !ok {
				t.Fatalf("%s: %s has no find rule", slug, a.Emulator)
			}
		}
	}
}
