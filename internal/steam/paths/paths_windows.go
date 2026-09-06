package paths

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

func candidates() []string {
	var out []string
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("SteamPath"); err == nil {
			out = append(out, filepath.Clean(v))
		}
		k.Close()
	}
	if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
		out = append(out, filepath.Join(pf, "Steam"))
	}
	return append(out, `C:\Program Files (x86)\Steam`)
}
