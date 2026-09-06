// Package artwork writes Steam's custom library art for a non-Steam shortcut.
//
// Steam reads these from userdata/<id>/config/grid/, named by the shortcut's
// 32-bit app id. Only the vertical capsule is written today: it is the image
// the library grid shows, and RomM's own cover fills it. Hero and logo need
// SteamGridDB, which RomM does not yet expose to clients.
package artwork

import (
	"fmt"
	"os"
	"path/filepath"
)

// Asset names the suffix Steam expects for each artwork slot.
type Asset string

const (
	// Capsule is the vertical box art shown in the library grid.
	Capsule Asset = "p"
	// Hero is the wide banner on the game's own page.
	Hero Asset = "_hero"
	// Logo is the transparent title treatment drawn over the hero.
	Logo Asset = "_logo"
)

// FileName is the name Steam looks for, e.g. "2147495993p.png".
func FileName(appID uint32, asset Asset) string {
	return fmt.Sprintf("%d%s.png", appID, asset)
}

// Write stores one artwork asset in gridDir, creating the directory if needed.
func Write(gridDir string, appID uint32, asset Asset, png []byte) error {
	if err := os.MkdirAll(gridDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(gridDir, FileName(appID, asset))
	tmp := dest + ".part"
	if err := os.WriteFile(tmp, png, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// Remove deletes every asset for a shortcut. Missing files are not an error:
// artwork is best-effort, so a shortcut may never have had any.
func Remove(gridDir string, appID uint32) error {
	for _, asset := range []Asset{Capsule, Hero, Logo} {
		if err := os.Remove(filepath.Join(gridDir, FileName(appID, asset))); err != nil &&
			!os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
