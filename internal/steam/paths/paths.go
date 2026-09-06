// Package paths locates the Steam installation and the active user's data
// directory on each operating system.
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// ErrNotFound is returned when no Steam installation can be located.
var ErrNotFound = errors.New("steam: installation not found")

// Install describes a Steam installation.
type Install struct {
	Root string // e.g. ~/.local/share/Steam or C:\Program Files (x86)\Steam
}

// Find locates Steam, honouring an explicit override first.
func Find(override string) (Install, error) {
	if override != "" {
		if isSteamRoot(override) {
			return Install{Root: override}, nil
		}
		return Install{}, ErrNotFound
	}
	for _, c := range candidates() {
		if isSteamRoot(c) {
			return Install{Root: c}, nil
		}
	}
	return Install{}, ErrNotFound
}

// UserDataDirs lists userdata/<steamid> directories, most recently used first.
// Recency is judged by the modification time of config/localconfig.vdf, which
// Steam rewrites on every session.
func (in Install) UserDataDirs() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(in.Root, "userdata"))
	if err != nil {
		return nil, err
	}
	type cand struct {
		dir string
		mod int64
	}
	var cands []cand
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil || e.Name() == "0" {
			continue
		}
		dir := filepath.Join(in.Root, "userdata", e.Name())
		var mod int64
		if st, err := os.Stat(filepath.Join(dir, "config", "localconfig.vdf")); err == nil {
			mod = st.ModTime().UnixNano()
		}
		cands = append(cands, cand{dir, mod})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod > cands[j].mod })
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.dir
	}
	return out, nil
}

// ShortcutsFile is the path of shortcuts.vdf inside a userdata directory.
func ShortcutsFile(userDataDir string) string {
	return filepath.Join(userDataDir, "config", "shortcuts.vdf")
}

// GridDir is where Steam reads custom artwork for a user.
func GridDir(userDataDir string) string {
	return filepath.Join(userDataDir, "config", "grid")
}

func isSteamRoot(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "userdata"))
	return err == nil && st.IsDir()
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}
