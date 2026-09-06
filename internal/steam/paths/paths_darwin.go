package paths

import "path/filepath"

func candidates() []string {
	return []string{filepath.Join(home(), "Library", "Application Support", "Steam")}
}
