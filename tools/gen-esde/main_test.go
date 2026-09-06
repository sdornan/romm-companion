package main

import (
	"reflect"
	"testing"
)

func TestParseCommandKeepsAStandaloneLaunch(t *testing.T) {
	alt, ok := parseCommand("PCSX2 (Standalone)", "%EMULATOR_PCSX2% -batch %ROM%")
	if !ok {
		t.Fatal("want the alternative kept")
	}
	if alt.Emulator != "PCSX2" || !reflect.DeepEqual(alt.Args, []string{"-batch", "%ROM%"}) {
		t.Fatalf("parsed: %+v", alt)
	}
}

func TestParseCommandReadsTheStartDirective(t *testing.T) {
	alt, ok := parseCommand(
		"MAME (Standalone)",
		`%STARTDIR%=~/.mame %EMULATOR_MAME% -rompath %GAMEDIR%\;%ROMPATH%/cps3 %BASENAME%`,
	)
	if !ok {
		t.Fatal("want the alternative kept")
	}
	if alt.StartDir != "~/.mame" {
		t.Errorf("start directory: %q", alt.StartDir)
	}
	// The escaped semicolon is part of the argument, not a token break.
	if want := []string{"-rompath", "%GAMEDIR%;%ROMPATH%/cps3", "%BASENAME%"}; !reflect.DeepEqual(alt.Args, want) {
		t.Errorf("args: %q", alt.Args)
	}
}

func TestParseCommandDropsWhatTheLauncherCannotHonour(t *testing.T) {
	for name, cmd := range map[string]string{
		"retroarch":  "%EMULATOR_RETROARCH% -L %CORE_RETROARCH%/snes9x_libretro.so %ROM%",
		"shell":      "%EMULATOR_OS-SHELL% %ROM%",
		"inject":     "%INJECT%=%BASENAME%.commands %EMULATOR_DOSBOX-X% %ROM%",
		"precommand": "%PRECOMMAND_WINE% %EMULATOR_DEMUL% -run=dc %ROM%",
		"gameentry":  "%EMULATOR_ARES% %GAMEENTRYDIR% %ROM%",
	} {
		if _, ok := parseCommand(name, cmd); ok {
			t.Errorf("%s should have been dropped", name)
		}
	}
}

func TestParseCommandDropsWindowOnlyDirectives(t *testing.T) {
	alt, ok := parseCommand("Dolphin", "%RUNINBACKGROUND% %HIDEWINDOW% %EMULATOR_DOLPHIN% -b -e %ROM%")
	if !ok {
		t.Fatal("want the alternative kept")
	}
	if want := []string{"-b", "-e", "%ROM%"}; !reflect.DeepEqual(alt.Args, want) {
		t.Fatalf("args: %q", alt.Args)
	}
}

func TestSplitCommandHonoursQuotesAndEscapes(t *testing.T) {
	got := splitCommand(`a "two words" c\ d e`)
	if want := []string{"a", "two words", "c d", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}
