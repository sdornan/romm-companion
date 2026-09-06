package esde

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Finder locates an emulator from its ES-DE find rule.
type Finder struct {
	// LookPath is exec.LookPath unless replaced in tests.
	LookPath func(string) (string, error)
	// Glob is filepath.Glob unless replaced in tests.
	Glob func(string) ([]string, error)
	// Home expands a leading "~"; empty falls back to the user's home directory.
	Home string
	// AppDir stands in for ES-DE's %ESPATH%, the directory holding the
	// running binary, which is where a portable Windows install keeps its
	// bundled emulators.
	AppDir string

	cache map[string]string
}

// NewFinder returns a Finder wired to the real filesystem.
func NewFinder() *Finder {
	f := &Finder{LookPath: exec.LookPath, Glob: filepath.Glob}
	f.Home, _ = os.UserHomeDir()
	if exe, err := os.Executable(); err == nil {
		f.AppDir = filepath.Dir(exe)
	}
	return f
}

// Find returns the executable for name, or "" when it is not installed.
// Results are memoised: resolution runs over every platform at startup, and
// the same emulator serves many of them.
func (f *Finder) Find(name string, rule FindRule) string {
	if f.cache == nil {
		f.cache = map[string]string{}
	}
	if p, ok := f.cache[name]; ok {
		return p
	}
	p := f.search(rule)
	f.cache[name] = p
	return p
}

func (f *Finder) search(rule FindRule) string {
	for _, entry := range rule.SystemPath {
		if p, err := f.LookPath(entry); err == nil {
			return p
		}
	}
	for _, entry := range rule.StaticPath {
		if p := f.matchStatic(entry); p != "" {
			return p
		}
	}
	return ""
}

// matchStatic expands one staticpath entry and returns the first executable
// it names. Entries may be globs, so the first match wins.
func (f *Finder) matchStatic(entry string) string {
	entry = f.expand(entry)
	if entry == "" {
		return ""
	}
	if !strings.ContainsAny(entry, "*?[") {
		if isExecutable(entry) {
			return entry
		}
		return ""
	}
	matches, err := f.Glob(entry)
	if err != nil {
		return ""
	}
	for _, m := range matches {
		if isExecutable(m) {
			return m
		}
	}
	return ""
}

// expand resolves the placeholders ES-DE uses in staticpath entries. An entry
// whose placeholder cannot be resolved is dropped rather than tried literally.
func (f *Finder) expand(entry string) string {
	entry = strings.ReplaceAll(entry, `\`, string(filepath.Separator))
	if strings.Contains(entry, "%ESPATH%") {
		if f.AppDir == "" {
			return ""
		}
		entry = strings.ReplaceAll(entry, "%ESPATH%", f.AppDir)
	}
	if strings.Contains(entry, "%") {
		return ""
	}
	if rest, ok := strings.CutPrefix(entry, "~"); ok {
		if f.Home == "" {
			return ""
		}
		entry = filepath.Join(f.Home, rest)
	}
	return filepath.Clean(entry)
}

// isExecutable reports whether path is a file this user could run. On Windows
// the mode bits carry no permission information, so existence is the test.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}
