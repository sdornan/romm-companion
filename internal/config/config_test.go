package config

import (
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	t.Setenv("ROMM_COMPANION_CONFIG", filepath.Join(t.TempDir(), "cfg", "config.json"))

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Paired() {
		t.Fatal("fresh config should not be paired")
	}
	c.ServerURL = "https://romm.example"
	c.Token = "secret"
	c.Templates["snes"] = `retroarch -L snes9x "%ROM%"`
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	back, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !back.Paired() || back.Templates["snes"] == "" {
		t.Fatalf("round trip lost fields: %+v", back)
	}
}
