// Package emulator decides which program opens a ROM for a given platform on
// this machine.
//
// Resolution order, first match wins:
//  1. the user's explicit template for the platform
//  2. a detected standalone emulator (not yet implemented; sourced from ES-DE's
//     system definitions and find rules in a later change)
//  3. RetroArch with the core RomM's own core map names for the platform
//  4. the RomM web player in an app-mode browser
//  5. nothing, which the capability report surfaces to RomM
package emulator

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Resolution is what the companion reports to RomM as launch_capabilities and
// what the launcher executes.
type Resolution struct {
	// Kind is "template", "standalone", "retroarch", "web_player" or "".
	Kind string
	// Descriptor is the short form sent to RomM, e.g. "retroarch:snes9x".
	Descriptor string
	// Command is the shell-free argv template. %ROM% is replaced at launch.
	Command []string
}

// Supported reports whether a launch is possible.
func (r Resolution) Supported() bool { return r.Kind != "" }

// Resolver holds the inputs resolution needs.
type Resolver struct {
	// Templates are user overrides keyed by platform slug.
	Templates map[string]string
	// Cores maps platform slug to libretro core names, best first. It comes
	// from RomM's own EmulatorJS core map so the two never drift.
	Cores map[string][]string
	// RetroArch is the resolved RetroArch executable, or empty if absent.
	RetroArch string
	// CoreDir is where RetroArch cores live, or empty to let RetroArch decide.
	CoreDir string
	// WebPlayerURL, when set, enables the web player fallback for every platform.
	WebPlayerURL string
	// LookPath is exec.LookPath unless replaced in tests.
	LookPath func(string) (string, error)
}

// NewResolver detects RetroArch on PATH.
func NewResolver(templates map[string]string, cores map[string][]string) *Resolver {
	r := &Resolver{Templates: templates, Cores: cores, LookPath: exec.LookPath}
	r.RetroArch = r.findRetroArch()
	return r
}

// Resolve picks the launch method for a platform slug.
func (r *Resolver) Resolve(slug string) Resolution {
	if t, ok := r.Templates[slug]; ok && strings.TrimSpace(t) != "" {
		return Resolution{Kind: "template", Descriptor: "template", Command: splitTemplate(t)}
	}
	if r.RetroArch != "" {
		if cores := r.Cores[slug]; len(cores) > 0 {
			core := cores[0]
			return Resolution{
				Kind:       "retroarch",
				Descriptor: "retroarch:" + core,
				Command:    []string{r.RetroArch, "-L", r.corePath(core), "%ROM%"},
			}
		}
	}
	if r.WebPlayerURL != "" {
		return Resolution{Kind: "web_player", Descriptor: "web_player"}
	}
	return Resolution{}
}

// Capabilities resolves every platform RomM knows about into the map the
// companion reports on registration.
func (r *Resolver) Capabilities(slugs []string) map[string]*string {
	out := make(map[string]*string, len(slugs))
	for _, s := range slugs {
		res := r.Resolve(s)
		if res.Supported() {
			d := res.Descriptor
			out[s] = &d
		} else {
			out[s] = nil
		}
	}
	return out
}

func (r *Resolver) findRetroArch() string {
	names := []string{"retroarch"}
	if runtime.GOOS == "windows" {
		names = []string{"retroarch.exe"}
	}
	for _, n := range names {
		if p, err := r.LookPath(n); err == nil {
			return p
		}
	}
	// Flatpak is the common Linux install; exec through flatpak run.
	if runtime.GOOS == "linux" {
		if fp, err := r.LookPath("flatpak"); err == nil {
			if exec.Command(fp, "info", "org.libretro.RetroArch").Run() == nil {
				return fp + " run org.libretro.RetroArch"
			}
		}
	}
	return ""
}

func (r *Resolver) corePath(core string) string {
	ext := map[string]string{"windows": ".dll", "darwin": ".dylib"}[runtime.GOOS]
	if ext == "" {
		ext = ".so"
	}
	name := core + "_libretro" + ext
	if r.CoreDir == "" {
		return name
	}
	return fmt.Sprintf("%s/%s", strings.TrimRight(r.CoreDir, "/"), name)
}

// splitTemplate splits a user template on whitespace while keeping quoted
// segments together, so `retroarch -L "my core.so" %ROM%` becomes four args.
func splitTemplate(t string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, ch := range t {
		switch {
		case ch == '"':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			flush()
		default:
			cur.WriteRune(ch)
		}
	}
	flush()
	return out
}
