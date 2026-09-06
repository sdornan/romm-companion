package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/sdornan/romm-companion/internal/autostart"
)

func cmdService(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: service <install|uninstall|status>")
	}
	switch args[0] {
	case "install":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		status, err := autostart.Install(exe)
		if status.Installed {
			fmt.Printf("Starts at login. Registered at %s\n", status.Path)
		}
		return err
	case "uninstall":
		if err := autostart.Uninstall(); err != nil {
			return err
		}
		fmt.Println("No longer starts at login.")
		return nil
	case "status":
		status, err := autostart.Current()
		if err != nil {
			return err
		}
		if status.Installed {
			fmt.Printf("Starts at login (%s)\n", status.Path)
		} else {
			fmt.Printf("Does not start at login. Run: romm-companion service install\n")
		}
		return nil
	default:
		return fmt.Errorf("unknown service subcommand %q", args[0])
	}
}
