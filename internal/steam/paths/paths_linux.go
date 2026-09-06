package paths

import "path/filepath"

func candidates() []string {
	h := home()
	return []string{
		filepath.Join(h, ".steam", "steam"),
		filepath.Join(h, ".local", "share", "Steam"),
		filepath.Join(h, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
		filepath.Join(h, "snap", "steam", "common", ".local", "share", "Steam"),
	}
}
