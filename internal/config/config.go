// Package config persists the companion's settings: which RomM server it is
// paired with, its device identity, and per-platform launch overrides.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is the on-disk settings file.
type Config struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	DeviceID  string `json:"device_id"`
	// DownloadDir is where ROM files are stored, one subdirectory per platform slug.
	DownloadDir string `json:"download_dir"`
	// SteamRoot overrides Steam installation discovery when set.
	SteamRoot string `json:"steam_root,omitempty"`
	// SteamUserDataDir pins a userdata/<steamid> directory when several exist.
	SteamUserDataDir string `json:"steam_user_data_dir,omitempty"`
	// Templates are user overrides of the launch command per platform slug.
	// They take precedence over every detected emulator.
	Templates map[string]string `json:"templates,omitempty"`
	// DeleteOnRemove also deletes the downloaded ROM when a shortcut is
	// removed. Off by default: the download directory may be shared with
	// another frontend that still lists the game.
	DeleteOnRemove bool `json:"delete_on_remove"`
}

// Path returns the settings file location, honouring ROMM_COMPANION_CONFIG.
func Path() (string, error) {
	if p := os.Getenv("ROMM_COMPANION_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "romm-companion", "config.json"), nil
}

// Load reads the settings file. A missing file yields defaults, not an error.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	c := defaults()
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Save writes the settings file atomically with owner-only permissions,
// since it holds the API token.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Paired reports whether the companion has completed pairing.
func (c *Config) Paired() bool {
	return c.ServerURL != "" && c.Token != ""
}

func defaults() *Config {
	c := &Config{Templates: map[string]string{}}
	if home, err := os.UserHomeDir(); err == nil {
		c.DownloadDir = filepath.Join(home, "RomM")
	}
	return c
}
