package cores

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/sdornan/romm-companion/internal/romm"
)

type fakeFetcher struct {
	cfg *romm.ServerConfig
	err error
}

func (f fakeFetcher) GetConfig(context.Context) (*romm.ServerConfig, error) {
	return f.cfg, f.err
}

// isolate points config.Path at a temporary directory so the cache written by
// a test never touches the developer's real settings directory.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("ROMM_COMPANION_CONFIG", filepath.Join(t.TempDir(), "config.json"))
}

func TestLoadMergesNightlyBehindStable(t *testing.T) {
	isolate(t)
	f := fakeFetcher{cfg: &romm.ServerConfig{
		EJSCores:        map[string][]string{"snes": {"snes9x"}},
		EJSNightlyCores: map[string][]string{"snes": {"bsnes_hd"}, "3ds": {"citra"}},
	}}

	m, err := Load(context.Background(), f)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := m["snes"]; len(got) != 1 || got[0] != "snes9x" {
		t.Errorf("nightly overrode stable: %v", got)
	}
	if got := m["3ds"]; len(got) != 1 || got[0] != "citra" {
		t.Errorf("nightly-only platform missing: %v", got)
	}
}

func TestLoadFallsBackToTheCachedCopy(t *testing.T) {
	isolate(t)
	good := fakeFetcher{cfg: &romm.ServerConfig{EJSCores: map[string][]string{"nes": {"fceumm"}}}}
	if _, err := Load(context.Background(), good); err != nil {
		t.Fatalf("priming the cache: %v", err)
	}

	offline := fakeFetcher{err: errors.New("dial tcp: connection refused")}
	m, err := Load(context.Background(), offline)
	if err == nil {
		t.Error("want the fetch error reported alongside the cache")
	}
	if got := m["nes"]; len(got) != 1 || got[0] != "fceumm" {
		t.Errorf("cache not used: %v", got)
	}
}

func TestLoadWithoutServerOrCacheFails(t *testing.T) {
	isolate(t)
	m, err := Load(context.Background(), fakeFetcher{err: errors.New("offline")})
	if err == nil || m != nil {
		t.Fatalf("want a failure with no map, got m=%v err=%v", m, err)
	}
}

func TestLoadTreatsAnEmptyServerMapAsAFailure(t *testing.T) {
	isolate(t)
	m, err := Load(context.Background(), fakeFetcher{cfg: &romm.ServerConfig{}})
	if err == nil || m != nil {
		t.Fatalf("want a failure with no map, got m=%v err=%v", m, err)
	}
}
