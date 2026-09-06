// Package cores holds RomM's platform-to-libretro-core map, fetched from the
// server so the companion and the web player always agree on which core runs a
// platform. The last good copy is cached on disk, so a launch still resolves
// when RomM is unreachable.
package cores

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/romm"
)

// Map is platform slug to libretro core names, best first.
type Map map[string][]string

// Fetcher is the part of romm.Client this package needs.
type Fetcher interface {
	GetConfig(ctx context.Context) (*romm.ServerConfig, error)
}

// Load returns the core map from the server, falling back to the cached copy
// when the server cannot be reached. It reports the fetch error alongside a
// usable cache so the caller can warn without failing the command.
func Load(ctx context.Context, f Fetcher) (Map, error) {
	cfg, err := f.GetConfig(ctx)
	if err == nil {
		m := merge(cfg)
		if len(m) > 0 {
			_ = writeCache(m)
			return m, nil
		}
		err = errors.New("romm: server returned an empty core map")
	}
	cached, cacheErr := readCache()
	if cacheErr != nil {
		return nil, err
	}
	return cached, err
}

// merge folds the nightly cores in behind the stable ones. Nightly builds are
// a superset, and resolution only needs the union.
func merge(cfg *romm.ServerConfig) Map {
	m := make(Map, len(cfg.EJSCores)+len(cfg.EJSNightlyCores))
	for slug, list := range cfg.EJSCores {
		m[slug] = list
	}
	for slug, list := range cfg.EJSNightlyCores {
		if _, ok := m[slug]; !ok {
			m[slug] = list
		}
	}
	return m
}

// cachePath is a file next to the settings file.
func cachePath() (string, error) {
	p, err := config.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "cores.json"), nil
}

func readCache() (Map, error) {
	p, err := cachePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var m Map
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if len(m) == 0 {
		return nil, errors.New("cores: cache is empty")
	}
	return m, nil
}

func writeCache(m Map) error {
	p, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
