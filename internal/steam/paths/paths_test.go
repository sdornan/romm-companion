package paths

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindOverrideAndUserDataOrdering(t *testing.T) {
	root := t.TempDir()
	if _, err := Find(root); err == nil {
		t.Fatal("dir without userdata should not be a Steam root")
	}
	for _, id := range []string{"111", "222", "0", "notanumber"} {
		if err := os.MkdirAll(filepath.Join(root, "userdata", id, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := filepath.Join(root, "userdata", "111", "config", "localconfig.vdf")
	recent := filepath.Join(root, "userdata", "222", "config", "localconfig.vdf")
	for _, p := range []string{old, recent} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	in, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := in.UserDataDirs()
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 || filepath.Base(dirs[0]) != "222" {
		t.Fatalf("want [222 111], got %v", dirs)
	}
	if ShortcutsFile(dirs[0]) != filepath.Join(dirs[0], "config", "shortcuts.vdf") {
		t.Fatal("ShortcutsFile path")
	}
}
