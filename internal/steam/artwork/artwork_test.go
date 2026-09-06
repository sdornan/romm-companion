package artwork

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNamesFilesTheWaySteamExpects(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "grid")
	if err := Write(dir, 2147495993, Capsule, []byte("png")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2147495993p.png")); err != nil {
		t.Fatalf("capsule not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2147495993p.png.part")); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
	if FileName(1, Hero) != "1_hero.png" || FileName(1, Logo) != "1_logo.png" {
		t.Fatal("asset suffixes")
	}
}

func TestRemoveIgnoresMissingAssets(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, 7, Capsule, []byte("png")); err != nil {
		t.Fatal(err)
	}
	if err := Remove(dir, 7); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := Remove(dir, 7); err != nil {
		t.Fatalf("second remove should be a no-op: %v", err)
	}
}
