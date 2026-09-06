package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/steam/paths"
	"github.com/sdornan/romm-companion/internal/steam/shortcuts"
)

func cmdSteam(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: steam <paths|list>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	install, err := paths.Find(cfg.SteamRoot)
	if err != nil {
		return fmt.Errorf("%w (set steam_root in the config file to override)", err)
	}
	userDirs, err := install.UserDataDirs()
	if err != nil {
		return err
	}
	switch args[0] {
	case "paths":
		fmt.Println("Steam root:   ", install.Root)
		for i, d := range userDirs {
			marker := ""
			if i == 0 {
				marker = "  (active)"
			}
			fmt.Println("User data:    ", d+marker)
		}
		return nil
	case "list":
		userDir, err := pickUserDir(cfg, userDirs)
		if err != nil {
			return err
		}
		f, err := os.Open(paths.ShortcutsFile(userDir))
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("No shortcuts.vdf yet; Steam has no non-Steam games for this user.")
			return nil
		}
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		sf, err := shortcuts.Read(f)
		if err != nil {
			return err
		}
		owned := sf.Owned()
		fmt.Printf("%d shortcuts in file, %d owned by RomM Companion\n", sf.Len(), len(owned))
		for _, s := range owned {
			fmt.Printf("  rom %-6d appid %d  %s\n", s.RomID, s.AppID, s.AppName)
		}
		return nil
	default:
		return fmt.Errorf("unknown steam subcommand %q", args[0])
	}
}

func pickUserDir(cfg *config.Config, dirs []string) (string, error) {
	if cfg.SteamUserDataDir != "" {
		return cfg.SteamUserDataDir, nil
	}
	if len(dirs) == 0 {
		return "", errors.New("no Steam user data directory found; log in to Steam once")
	}
	return dirs[0], nil
}
